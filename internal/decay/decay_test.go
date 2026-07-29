package decay_test

import (
	"context"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/clock"
	"github.com/yaad-index/yaadegar/internal/decay"
	"github.com/yaad-index/yaadegar/internal/email"
	"github.com/yaad-index/yaadegar/internal/storage"
	"github.com/yaad-index/yaadegar/internal/storage/sqlstore"
)

const giverEmail = "giver@example.com"

type fixture struct {
	store    storage.Store
	ts       storage.TenantStore
	item     storage.Item
	resID    string
	sweeper  *decay.Sweeper
	fakeMail *email.FakeSender
	clk      *clock.Fake
}

// setup seeds a tenant/owner/list(decayDays override)/item/reservation at t0 and
// wires a sweeper with a fake clock, a fake mailer, the given instance default
// period, and a 24h response window.
func setup(t *testing.T, t0 time.Time, listDecayDays *int, instanceDefaultDays int) *fixture {
	t.Helper()
	ctx := context.Background()
	dsn := "file:" + filepath.Join(t.TempDir(), "decay.db")
	store, err := sqlstore.Open(ctx, storage.Config{Driver: storage.DriverSQLite, DSN: dsn})
	require.NoError(t, err)
	require.NoError(t, store.Migrate(ctx))
	t.Cleanup(func() { _ = store.Close() })

	tenant, err := store.CreateTenant(ctx, storage.Tenant{Subdomain: "alice"})
	require.NoError(t, err)
	ts := store.ForTenant(tenant)
	owner, err := ts.Users().Create(ctx, storage.User{Name: "Alice", Email: "owner@example.com"})
	require.NoError(t, err)
	list, err := ts.Lists().Create(ctx, storage.List{Title: "List", DecayDays: listDecayDays, Active: true}, owner.ID)
	require.NoError(t, err)
	item, err := ts.Items().Create(ctx, storage.Item{ListID: list.ID, Name: "Blender", QuantityWanted: 1})
	require.NoError(t, err)
	res, err := ts.Reservations().Create(ctx, storage.Reservation{
		ItemID: item.ID, GiverEmail: ptr(giverEmail), Quantity: 1, TokenHash: "cap", CreatedAt: t0,
	})
	require.NoError(t, err)

	fakeMail := &email.FakeSender{}
	clk := clock.NewFake(t0)
	sweeper := decay.NewSweeper(store, fakeMail, clk, decay.Config{
		DefaultDecayDays: instanceDefaultDays,
		ResponseWindow:   24 * time.Hour,
		LinkBase:         "https://alice.example.test",
	}, slog.New(slog.DiscardHandler))

	return &fixture{store: store, ts: ts, item: item, resID: res.ID, sweeper: sweeper, fakeMail: fakeMail, clk: clk}
}

func ptr[T any](v T) *T { return &v }

// failingSender always fails, standing in for a transient SMTP outage.
type failingSender struct{ calls int }

func (f *failingSender) Send(context.Context, email.Message) error {
	f.calls++
	return assert.AnError
}

func (f *fixture) state(t *testing.T) storage.ReservationState {
	t.Helper()
	r, err := f.ts.Reservations().Get(context.Background(), f.resID)
	require.NoError(t, err)
	return r.State
}

// TestDecayEscalation: list overrides decay to 30 days → reserver notified once
// with both links, then auto-expired after the response window; anonymity and
// idempotency hold, and no owner is ever emailed.
func TestDecayEscalation(t *testing.T) {
	ctx := context.Background()
	t0 := time.Date(2027, 1, 1, 12, 0, 0, 0, time.UTC)
	f := setup(t, t0, ptr(30), 0)

	require.NoError(t, f.sweeper.Sweep(ctx))
	assert.Equal(t, storage.StateActive, f.state(t))
	assert.Empty(t, f.fakeMail.Sent())

	// Past the 30-day period → reserver notified once, with keep + release links.
	f.clk.Set(t0.Add(31 * 24 * time.Hour))
	require.NoError(t, f.sweeper.Sweep(ctx))
	assert.Equal(t, storage.StateReserverNotified, f.state(t))
	sent := f.fakeMail.Sent()
	require.Len(t, sent, 1)
	assert.Equal(t, giverEmail, sent[0].To)
	assert.Contains(t, sent[0].Body, "Blender")
	assert.Contains(t, sent[0].Body, "/decay-keep?token=")
	assert.Contains(t, sent[0].Body, "/decay-release?token=")
	assert.NotContains(t, sent[0].Body, "owner@example.com", "owner is never contacted")

	// Idempotent: re-sweeping before the window elapses sends nothing.
	require.NoError(t, f.sweeper.Sweep(ctx))
	assert.Len(t, f.fakeMail.Sent(), 1)

	// After the 24h response window → auto-expire; no email; item freed.
	f.clk.Advance(24 * time.Hour)
	require.NoError(t, f.sweeper.Sweep(ctx))
	assert.Equal(t, storage.StateExpired, f.state(t))
	assert.Len(t, f.fakeMail.Sent(), 1, "auto-expiry sends no email")

	qty, err := f.ts.Items().ReservedQuantity(ctx, f.item.ID)
	require.NoError(t, err)
	assert.Zero(t, qty, "auto-expiry frees the item")

	require.NoError(t, f.sweeper.Sweep(ctx)) // terminal
	assert.Len(t, f.fakeMail.Sent(), 1)
}

// TestDecaySendFailureHoldsActive: a transient send failure must NOT advance the
// reservation — it stays active and is retried on the next sweep (#39), so a
// reserver is never silently expired without notice. Once the send succeeds, the
// state advances.
func TestDecaySendFailureHoldsActive(t *testing.T) {
	ctx := context.Background()
	t0 := time.Date(2027, 1, 1, 12, 0, 0, 0, time.UTC)
	f := setup(t, t0, ptr(30), 0)

	// Rewire the sweeper with a failing sender against the same store.
	failer := &failingSender{}
	sweeper := decay.NewSweeper(f.store, failer, f.clk, decay.Config{
		DefaultDecayDays: 0,
		ResponseWindow:   24 * time.Hour,
		LinkBase:         "https://alice.example.test",
	}, slog.New(slog.DiscardHandler))

	f.clk.Set(t0.Add(31 * 24 * time.Hour))
	require.NoError(t, sweeper.Sweep(ctx)) // one failed candidate is logged, not fatal
	assert.Equal(t, storage.StateActive, f.state(t), "send failure must not advance state")
	assert.Positive(t, failer.calls)

	// A subsequent sweep with a working sender advances and notifies.
	require.NoError(t, f.sweeper.Sweep(ctx))
	assert.Equal(t, storage.StateReserverNotified, f.state(t))
	require.Len(t, f.fakeMail.Sent(), 1)
}

// TestDecayInheritsInstanceDefault: list override is nil, instance default is 30
// → decay fires off the instance default (the resolver path).
func TestDecayInheritsInstanceDefault(t *testing.T) {
	ctx := context.Background()
	t0 := time.Date(2027, 1, 1, 12, 0, 0, 0, time.UTC)
	f := setup(t, t0, nil, 30)

	f.clk.Set(t0.Add(31 * 24 * time.Hour))
	require.NoError(t, f.sweeper.Sweep(ctx))
	assert.Equal(t, storage.StateReserverNotified, f.state(t))
	assert.Len(t, f.fakeMail.Sent(), 1)
}

// TestDecayExplicitOff: list overrides to 0 (off) even though the instance
// default is 30 → never decays.
func TestDecayExplicitOff(t *testing.T) {
	ctx := context.Background()
	t0 := time.Date(2027, 1, 1, 12, 0, 0, 0, time.UTC)
	f := setup(t, t0, ptr(0), 30)

	f.clk.Set(t0.Add(365 * 24 * time.Hour))
	require.NoError(t, f.sweeper.Sweep(ctx))
	assert.Equal(t, storage.StateActive, f.state(t))
	assert.Empty(t, f.fakeMail.Sent())
}

// TestDecayInheritsOffDefault: list override nil and instance default 0 (the
// shipped default) → never decays.
func TestDecayInheritsOffDefault(t *testing.T) {
	ctx := context.Background()
	t0 := time.Date(2027, 1, 1, 12, 0, 0, 0, time.UTC)
	f := setup(t, t0, nil, 0)

	f.clk.Set(t0.Add(365 * 24 * time.Hour))
	require.NoError(t, f.sweeper.Sweep(ctx))
	assert.Equal(t, storage.StateActive, f.state(t))
	assert.Empty(t, f.fakeMail.Sent())
}

// pendingFixture rewires the fixture's sweeper with a confirm window and creates a
// pending_confirmation reservation at t0 alongside the setup's active one.
func (f *fixture) withConfirmWindow(t *testing.T, t0 time.Time, decayDefaultDays int, confirmWindow time.Duration) string {
	t.Helper()
	f.sweeper = decay.NewSweeper(f.store, f.fakeMail, f.clk, decay.Config{
		DefaultDecayDays: decayDefaultDays,
		ResponseWindow:   24 * time.Hour,
		ConfirmWindow:    confirmWindow,
	}, slog.New(slog.DiscardHandler))
	pend, err := f.ts.Reservations().Create(context.Background(), storage.Reservation{
		ItemID: f.item.ID, GiverEmail: ptr(giverEmail), Quantity: 1, TokenHash: "cap-pend",
		ConfirmTokenHash: "conf-hash", State: storage.StatePendingConfirmation, CreatedAt: t0,
	})
	require.NoError(t, err)
	return pend.ID
}

func (f *fixture) stateOf(t *testing.T, id string) storage.ReservationState {
	t.Helper()
	r, err := f.ts.Reservations().Get(context.Background(), id)
	require.NoError(t, err)
	return r.State
}

// TestConfirmWindowExpiry (#81): a pending_confirmation reservation stays pending
// within the confirm window and auto-expires past it — silently (no email).
func TestConfirmWindowExpiry(t *testing.T) {
	ctx := context.Background()
	t0 := time.Date(2027, 1, 1, 12, 0, 0, 0, time.UTC)
	f := setup(t, t0, nil, 0) // instance decay off; only the confirm window is exercised
	pendID := f.withConfirmWindow(t, t0, 0, 30*time.Minute)

	// Within the window → still pending, no email.
	f.clk.Set(t0.Add(20 * time.Minute))
	require.NoError(t, f.sweeper.Sweep(ctx))
	assert.Equal(t, storage.StatePendingConfirmation, f.stateOf(t, pendID))
	assert.Empty(t, f.fakeMail.Sent(), "confirm-window expiry sends no email")

	// Past the window → expired (frees the item).
	f.clk.Set(t0.Add(31 * time.Minute))
	require.NoError(t, f.sweeper.Sweep(ctx))
	assert.Equal(t, storage.StateExpired, f.stateOf(t, pendID))
	assert.Empty(t, f.fakeMail.Sent())
}

// TestPendingExcludedFromActiveDecay (#81, Addition B-ii): a pending reservation is
// eligible ONLY for confirm-window expiry — never the active-reservation decay
// path. With a short decay period but a long confirm window, advancing past the
// decay period leaves the pending reservation pending (not notified, not expired).
func TestPendingExcludedFromActiveDecay(t *testing.T) {
	ctx := context.Background()
	t0 := time.Date(2027, 1, 1, 12, 0, 0, 0, time.UTC)
	f := setup(t, t0, nil, 1)                             // instance decay = 1 day
	pendID := f.withConfirmWindow(t, t0, 1, 72*time.Hour) // confirm window 72h

	// 25h later: past the 1-day decay period but well within the 72h confirm window.
	f.clk.Set(t0.Add(25 * time.Hour))
	require.NoError(t, f.sweeper.Sweep(ctx))
	assert.Equal(t, storage.StatePendingConfirmation, f.stateOf(t, pendID),
		"a pending reservation is never advanced by the active-decay path")
}
