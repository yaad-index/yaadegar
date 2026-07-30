package cobuy_test

import (
	"context"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/clock"
	"github.com/yaad-index/yaadegar/internal/cobuy"
	"github.com/yaad-index/yaadegar/internal/storage"
	"github.com/yaad-index/yaadegar/internal/storage/sqlstore"
)

const window = 168 * time.Hour

type fixture struct {
	store storage.Store
	ts    storage.TenantStore
	item  storage.Item
	clk   *clock.Fake
}

func ptr[T any](v T) *T { return &v }

// newFixture seeds a tenant/owner/list/priced-item and returns a handle. The clock
// starts at t0; the sweeper is built per-test so the window can vary.
func newFixture(t *testing.T, t0 time.Time) *fixture {
	t.Helper()
	ctx := context.Background()
	dsn := "file:" + filepath.Join(t.TempDir(), "cobuy.db")
	store, err := sqlstore.Open(ctx, storage.Config{Driver: storage.DriverSQLite, DSN: dsn})
	require.NoError(t, err)
	require.NoError(t, store.Migrate(ctx))
	t.Cleanup(func() { _ = store.Close() })

	tenant, err := store.CreateTenant(ctx, storage.Tenant{Subdomain: "alice"})
	require.NoError(t, err)
	ts := store.ForTenant(tenant)
	owner, err := ts.Users().Create(ctx, storage.User{Name: "Alice"})
	require.NoError(t, err)
	list, err := ts.Lists().Create(ctx, storage.List{Title: "List", Active: true}, owner.ID)
	require.NoError(t, err)
	item, err := ts.Items().Create(ctx, storage.Item{
		ListID: list.ID, Name: "Espresso machine", QuantityWanted: 1,
		Price: &storage.Money{AmountMinor: 10000, Currency: "EUR"},
	})
	require.NoError(t, err)

	return &fixture{store: store, ts: ts, item: item, clk: clock.NewFake(t0)}
}

// proposeMatch creates two pledges covering the price and a proposed match linking
// them, stamped at proposedAt — the store-level shape the API's maybeProposeMatch
// produces, minus the emails.
func (f *fixture) proposeMatch(t *testing.T, proposedAt time.Time) (storage.Match, []string) {
	t.Helper()
	ctx := context.Background()
	var ids []string
	for i, email := range []string{"a@example.com", "b@example.com"} {
		c, err := f.ts.Contributions().Create(ctx, storage.Contribution{
			ItemID:       f.item.ID,
			Pledged:      storage.Money{AmountMinor: 5000, Currency: "EUR"},
			ContactEmail: email,
			TokenHash:    "cap" + string(rune('a'+i)),
			// A proposed match's pledges carry a scoped token; the sweep must clear it.
			MatchActionTokenHash:      "scoped" + string(rune('a'+i)),
			MatchActionTokenExpiresAt: ptr(proposedAt.Add(window)),
		})
		require.NoError(t, err)
		ids = append(ids, c.ID)
	}
	m, err := f.ts.Matches().Create(ctx, storage.Match{ItemID: f.item.ID, ContributionIDs: ids, CreatedAt: proposedAt})
	require.NoError(t, err)
	// Create stamps the linked contributions matched; confirm that baseline.
	require.Equal(t, storage.MatchProposed, m.State)
	return m, ids
}

func (f *fixture) newSweeper() *cobuy.Sweeper {
	return cobuy.NewSweeper(f.store, f.clk, window, slog.New(slog.DiscardHandler))
}

func (f *fixture) match(t *testing.T, id string) storage.Match {
	t.Helper()
	m, err := f.ts.Matches().Get(context.Background(), id)
	require.NoError(t, err)
	return m
}

func (f *fixture) contrib(t *testing.T, id string) storage.Contribution {
	t.Helper()
	c, err := f.ts.Contributions().Get(context.Background(), id)
	require.NoError(t, err)
	return c
}

// A proposed match past the confirm window is dissolved: match + all pledges go
// terminal (expired) and the scoped tokens are cleared.
func TestSweep_ExpiresProposedMatchPastWindow(t *testing.T) {
	ctx := context.Background()
	t0 := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	f := newFixture(t, t0)
	m, ids := f.proposeMatch(t, t0)

	// Advance just past the window and sweep.
	f.clk.Advance(window + time.Minute)
	require.NoError(t, f.newSweeper().Sweep(ctx))

	assert.Equal(t, storage.MatchExpired, f.match(t, m.ID).State)
	for _, id := range ids {
		c := f.contrib(t, id)
		assert.Equal(t, storage.ContributionExpired, c.Status, "pledge %s should be expired", id)
		assert.Empty(t, c.MatchActionTokenHash, "scoped token should be cleared")
		assert.Nil(t, c.MatchActionTokenExpiresAt, "scoped token expiry should be cleared")
		assert.Equal(t, m.ID, *c.MatchID, "match_id kept as the audit link")
	}
}

// A swept match frees the item for reservation (#93 interaction): with every pledge
// terminal, a reserve now succeeds where it would have been blocked while proposed.
func TestSweep_FreesItemForReserve(t *testing.T) {
	ctx := context.Background()
	t0 := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	f := newFixture(t, t0)
	f.proposeMatch(t, t0)

	// While proposed, a reserve is blocked by the live co-buy.
	_, err := f.ts.Reservations().CreateWithinCapacity(ctx,
		storage.Reservation{ItemID: f.item.ID, Quantity: 1, TokenHash: "r"}, 1)
	require.ErrorIs(t, err, storage.ErrCrossTrackConflict)

	f.clk.Advance(window + time.Minute)
	require.NoError(t, f.newSweeper().Sweep(ctx))

	// After the sweep the item is free — the reserve goes through.
	_, err = f.ts.Reservations().CreateWithinCapacity(ctx,
		storage.Reservation{ItemID: f.item.ID, Quantity: 1, TokenHash: "r2"}, 1)
	require.NoError(t, err)
}

// A proposed match still within the window is untouched.
func TestSweep_WithinWindowUntouched(t *testing.T) {
	ctx := context.Background()
	t0 := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	f := newFixture(t, t0)
	m, ids := f.proposeMatch(t, t0)

	f.clk.Advance(window - time.Minute)
	require.NoError(t, f.newSweeper().Sweep(ctx))

	assert.Equal(t, storage.MatchProposed, f.match(t, m.ID).State)
	for _, id := range ids {
		assert.Equal(t, storage.ContributionMatched, f.contrib(t, id).Status)
	}
}

// An already-resolved match is never swept — both_confirmed and declined are left
// exactly as they were even long past the window.
func TestSweep_ResolvedMatchNeverSwept(t *testing.T) {
	ctx := context.Background()
	t0 := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)

	for _, tc := range []struct {
		name  string
		state storage.MatchState
	}{
		{"both_confirmed", storage.MatchBothConfirmed},
		{"declined", storage.MatchDeclined},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, t0)
			m, _ := f.proposeMatch(t, t0)
			m.State = tc.state
			_, err := f.ts.Matches().Update(ctx, m)
			require.NoError(t, err)

			f.clk.Advance(window * 10)
			require.NoError(t, f.newSweeper().Sweep(ctx))

			assert.Equal(t, tc.state, f.match(t, m.ID).State, "resolved match must not be swept")
		})
	}
}

// A non-positive window disables the sweep entirely (matches never auto-expire,
// mirroring the never-expiring scoped token).
func TestSweep_ZeroWindowDisabled(t *testing.T) {
	ctx := context.Background()
	t0 := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	f := newFixture(t, t0)
	m, _ := f.proposeMatch(t, t0)

	f.clk.Advance(window * 10)
	s := cobuy.NewSweeper(f.store, f.clk, 0, slog.New(slog.DiscardHandler))
	require.NoError(t, s.Sweep(ctx))

	assert.Equal(t, storage.MatchProposed, f.match(t, m.ID).State)
}
