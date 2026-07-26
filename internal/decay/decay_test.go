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
	list, err := ts.Lists().Create(ctx, storage.List{OwnerID: owner.ID, Title: "List", DecayDays: listDecayDays, Active: true})
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

	return &fixture{ts: ts, item: item, resID: res.ID, sweeper: sweeper, fakeMail: fakeMail, clk: clk}
}

func ptr[T any](v T) *T { return &v }

func (f *fixture) state(t *testing.T) storage.ReservationDecayState {
	t.Helper()
	r, err := f.ts.Reservations().Get(context.Background(), f.resID)
	require.NoError(t, err)
	return r.DecayState
}

// TestDecayEscalation: list overrides decay to 30 days → reserver notified once
// with both links, then auto-expired after the response window; anonymity and
// idempotency hold, and no owner is ever emailed.
func TestDecayEscalation(t *testing.T) {
	ctx := context.Background()
	t0 := time.Date(2027, 1, 1, 12, 0, 0, 0, time.UTC)
	f := setup(t, t0, ptr(30), 0)

	require.NoError(t, f.sweeper.Sweep(ctx))
	assert.Equal(t, storage.DecayActive, f.state(t))
	assert.Empty(t, f.fakeMail.Sent())

	// Past the 30-day period → reserver notified once, with keep + release links.
	f.clk.Set(t0.Add(31 * 24 * time.Hour))
	require.NoError(t, f.sweeper.Sweep(ctx))
	assert.Equal(t, storage.DecayReserverNotified, f.state(t))
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
	assert.Equal(t, storage.DecayExpired, f.state(t))
	assert.Len(t, f.fakeMail.Sent(), 1, "auto-expiry sends no email")

	qty, err := f.ts.Items().ReservedQuantity(ctx, f.item.ID)
	require.NoError(t, err)
	assert.Zero(t, qty, "auto-expiry frees the item")

	require.NoError(t, f.sweeper.Sweep(ctx)) // terminal
	assert.Len(t, f.fakeMail.Sent(), 1)
}

// TestDecayInheritsInstanceDefault: list override is nil, instance default is 30
// → decay fires off the instance default (the resolver path).
func TestDecayInheritsInstanceDefault(t *testing.T) {
	ctx := context.Background()
	t0 := time.Date(2027, 1, 1, 12, 0, 0, 0, time.UTC)
	f := setup(t, t0, nil, 30)

	f.clk.Set(t0.Add(31 * 24 * time.Hour))
	require.NoError(t, f.sweeper.Sweep(ctx))
	assert.Equal(t, storage.DecayReserverNotified, f.state(t))
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
	assert.Equal(t, storage.DecayActive, f.state(t))
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
	assert.Equal(t, storage.DecayActive, f.state(t))
	assert.Empty(t, f.fakeMail.Sent())
}
