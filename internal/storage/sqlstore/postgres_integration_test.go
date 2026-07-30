//go:build integration

// Package sqlstore's Postgres integration test. It is guarded by the
// `integration` build tag so the default `check` CI (which has no database)
// stays green; run it explicitly against a live Postgres:
//
//	YAADEGAR_TEST_POSTGRES_DSN='postgres://user:pass@localhost:5432/yaadegar_test?sslmode=disable' \
//	    go test -tags=integration -race ./internal/storage/sqlstore/
//
// The Postgres driver shares the entire CRUD body with SQLite (ADR-0003 §1); this
// test exists to prove the Postgres dialect, placeholder rebinding, and migration
// SQL actually run against a real server.
package sqlstore_test

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/storage"
	"github.com/yaad-index/yaadegar/internal/storage/sqlstore"
)

func newPostgresStore(t *testing.T) storage.Store {
	t.Helper()
	dsn := os.Getenv("YAADEGAR_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set YAADEGAR_TEST_POSTGRES_DSN to run the Postgres integration test")
	}
	st, err := sqlstore.Open(context.Background(), storage.Config{
		Driver: storage.DriverPostgres,
		DSN:    dsn,
	})
	require.NoError(t, err)
	require.NoError(t, st.Migrate(context.Background()))
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// TestPostgres_RoundTripAndIsolation runs a representative slice of the contract
// against a real Postgres: a scoped CRUD round-trip, unique-conflict mapping, and
// the cross-tenant isolation guarantee.
func TestPostgres_RoundTripAndIsolation(t *testing.T) {
	ctx := context.Background()
	st := newPostgresStore(t)

	// Unique subdomains per run to tolerate a shared, non-reset database.
	suffix := t.Name()
	tenA, err := st.CreateTenant(ctx, storage.Tenant{Subdomain: "a-" + suffix})
	require.NoError(t, err)
	tenB, err := st.CreateTenant(ctx, storage.Tenant{Subdomain: "b-" + suffix})
	require.NoError(t, err)

	// Duplicate subdomain → ErrConflict (exercises pgconn 23505 mapping).
	_, err = st.CreateTenant(ctx, storage.Tenant{Subdomain: "a-" + suffix})
	assert.ErrorIs(t, err, storage.ErrConflict)

	as := st.ForTenant(tenA)
	bs := st.ForTenant(tenB)

	owner, err := as.Users().Create(ctx, storage.User{Name: "Alice"})
	require.NoError(t, err)
	list, err := as.Lists().Create(ctx, storage.List{
		Title: "PG list",
	}, owner.ID)
	require.NoError(t, err)
	item, err := as.Items().Create(ctx, storage.Item{
		ListID: list.ID,
		Name:   "Kettle",
		Price:  &storage.Money{AmountMinor: 3500, Currency: "GBP"},
	})
	require.NoError(t, err)

	got, err := as.Items().Get(ctx, item.ID)
	require.NoError(t, err)
	require.NotNil(t, got.Price)
	assert.Equal(t, int64(3500), got.Price.AmountMinor)

	// Cross-tenant isolation holds on Postgres too.
	_, err = bs.Lists().Get(ctx, list.ID)
	assert.ErrorIs(t, err, storage.ErrNotFound)
	_, err = bs.Items().Get(ctx, item.ID)
	assert.ErrorIs(t, err, storage.ErrNotFound)
}

// TestPostgres_ConcurrentConfirmSingleCompletion is the #36 guard on real
// Postgres: two truly-parallel confirms on one match (the SQLite unit test
// serializes via a single connection; here the pool is uncapped and the item lock
// is a genuine SELECT ... FOR UPDATE). Exactly one confirm must observe the
// completing transition, so the reveal fires once.
func TestPostgres_ConcurrentConfirmSingleCompletion(t *testing.T) {
	ctx := context.Background()
	st := newPostgresStore(t)
	suffix := t.Name()

	ten, err := st.CreateTenant(ctx, storage.Tenant{Subdomain: "cc-" + suffix})
	require.NoError(t, err)
	s := st.ForTenant(ten)
	owner, err := s.Users().Create(ctx, storage.User{Name: "Owner"})
	require.NoError(t, err)
	list, err := s.Lists().Create(ctx, storage.List{Title: "Co-buy"}, owner.ID)
	require.NoError(t, err)
	item, err := s.Items().Create(ctx, storage.Item{
		ListID: list.ID, Name: "Espresso machine",
		Price: &storage.Money{AmountMinor: 20000, Currency: "EUR"},
	})
	require.NoError(t, err)

	// Repeat many times over fresh matches — each round is a real parallel race, so
	// a missing lock would double-complete on at least one round.
	for round := 0; round < 40; round++ {
		c1, err := s.Contributions().Create(ctx, storage.Contribution{
			ItemID: item.ID, Pledged: storage.Money{AmountMinor: 10000, Currency: "EUR"},
			ContactEmail: "a@example.com", TokenHash: fmt.Sprintf("hash-%d-a", round),
		})
		require.NoError(t, err)
		c2, err := s.Contributions().Create(ctx, storage.Contribution{
			ItemID: item.ID, Pledged: storage.Money{AmountMinor: 10000, Currency: "EUR"},
			ContactEmail: "b@example.com", TokenHash: fmt.Sprintf("hash-%d-b", round),
		})
		require.NoError(t, err)
		match, err := s.Matches().Create(ctx, storage.Match{
			ItemID: item.ID, ContributionIDs: []string{c1.ID, c2.ID},
		})
		require.NoError(t, err)

		var (
			wg        sync.WaitGroup
			mu        sync.Mutex
			completed int
			start     = make(chan struct{})
		)
		for _, cid := range []string{c1.ID, c2.ID} {
			wg.Add(1)
			go func(cid string) {
				defer wg.Done()
				<-start // release both goroutines together
				_, _, done, err := s.Matches().ConfirmContribution(ctx, item.ID, match.ID, cid)
				assert.NoError(t, err)
				if done {
					mu.Lock()
					completed++
					mu.Unlock()
				}
			}(cid)
		}
		close(start)
		wg.Wait()

		require.Equal(t, 1, completed, "exactly one confirm completes the match (round %d)", round)
		final, err := s.Matches().Get(ctx, match.ID)
		require.NoError(t, err)
		assert.Equal(t, storage.MatchBothConfirmed, final.State)
	}
}

// TestPostgres_DomainReclaimConcurrentSingleWinner is the #42 guard on real
// Postgres: two tenants race to reclaim the same expired unverified hostname; the
// SELECT ... FOR UPDATE serializes them so exactly one wins and the hostname is
// never double-claimed.
func TestPostgres_DomainReclaimConcurrentSingleWinner(t *testing.T) {
	ctx := context.Background()
	st := newPostgresStore(t)
	suffix := t.Name()
	hostname := "reclaim-" + suffix + ".example.com"

	// A squatter parks the hostname, unverified, long ago.
	squat, err := st.CreateTenant(ctx, storage.Tenant{Subdomain: "sq-" + suffix})
	require.NoError(t, err)
	old := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	_, err = st.ForTenant(squat).Domains().Create(ctx, storage.Domain{
		Hostname: hostname, VerificationToken: "tok-squat", CreatedAt: old,
	})
	require.NoError(t, err)

	expiredBefore := old.Add(time.Hour) // the squatter's claim is past the window
	var claimants []storage.TenantStore
	for _, sub := range []string{"r1-" + suffix, "r2-" + suffix} {
		ten, err := st.CreateTenant(ctx, storage.Tenant{Subdomain: sub})
		require.NoError(t, err)
		claimants = append(claimants, st.ForTenant(ten))
	}

	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		wins  int
		start = make(chan struct{})
	)
	for i, cs := range claimants {
		wg.Add(1)
		go func(i int, cs storage.TenantStore) {
			defer wg.Done()
			<-start
			_, err := cs.Domains().CreateReclaimingExpired(ctx, storage.Domain{
				Hostname: hostname, VerificationToken: fmt.Sprintf("tok-%d", i),
				CreatedAt: expiredBefore.Add(time.Hour),
			}, expiredBefore)
			if err == nil {
				mu.Lock()
				wins++
				mu.Unlock()
			} else {
				assert.ErrorIs(t, err, storage.ErrConflict)
			}
		}(i, cs)
	}
	close(start)
	wg.Wait()

	assert.Equal(t, 1, wins, "exactly one tenant reclaims the expired hostname under real FOR UPDATE concurrency")
}

// TestPostgres_ConfirmVsExpireSingleWinner is the ADR-0007 confirm-race guard on
// real Postgres: a pending_confirmation reservation is raced by a giver confirm
// (→ active) and the confirm-window sweep (→ expired). The row-locked from-state
// transitions must let exactly one win, leaving the other a no-op — never both.
func TestPostgres_ConfirmVsExpireSingleWinner(t *testing.T) {
	ctx := context.Background()
	st := newPostgresStore(t)
	suffix := t.Name()

	ten, err := st.CreateTenant(ctx, storage.Tenant{Subdomain: "ce-" + suffix})
	require.NoError(t, err)
	s := st.ForTenant(ten)
	owner, err := s.Users().Create(ctx, storage.User{Name: "Owner"})
	require.NoError(t, err)
	list, err := s.Lists().Create(ctx, storage.List{Title: "Confirm"}, owner.ID)
	require.NoError(t, err)
	item, err := s.Items().Create(ctx, storage.Item{ListID: list.ID, Name: "Toaster", QuantityWanted: 1})
	require.NoError(t, err)

	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	for round := 0; round < 40; round++ {
		id := fmt.Sprintf("res-%s-%d", suffix, round)
		_, err := s.Reservations().CreateWithinCapacity(ctx, storage.Reservation{
			ID: id, ItemID: item.ID, Quantity: 1,
			TokenHash:        "pending:" + id,
			ConfirmTokenHash: fmt.Sprintf("confirm-%d", round),
			State:            storage.StatePendingConfirmation,
		}, item.QuantityWanted)
		require.NoError(t, err)

		var (
			wg              sync.WaitGroup
			mu              sync.Mutex
			confirmed, gone bool
			start           = make(chan struct{})
		)
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			ok, err := s.Reservations().ConfirmReservation(ctx, id, fmt.Sprintf("cap-%d", round), now)
			assert.NoError(t, err)
			if ok {
				mu.Lock()
				confirmed = true
				mu.Unlock()
			}
		}()
		go func() {
			defer wg.Done()
			<-start
			ok, err := s.Reservations().ExpirePending(ctx, id, now)
			assert.NoError(t, err)
			if ok {
				mu.Lock()
				gone = true
				mu.Unlock()
			}
		}()
		close(start)
		wg.Wait()

		// Exactly one transition wins; the other is a no-op.
		assert.Truef(t, confirmed != gone, "exactly one of confirm/expire wins (round %d: confirmed=%v gone=%v)", round, confirmed, gone)
		final, err := s.Reservations().Get(ctx, id)
		require.NoError(t, err)
		if confirmed {
			assert.Equal(t, storage.StateActive, final.State, "confirm won (round %d)", round)
			require.NotNil(t, final.EmailConfirmedAt)
		} else {
			assert.Equal(t, storage.StateExpired, final.State, "expire won (round %d)", round)
		}

		// Reset the item for the next round so capacity is free again.
		require.NoError(t, s.Reservations().Delete(ctx, id))
	}
}

// TestPostgres_ReserveCoBuyMutualExclusionRace is the #93 guard on real Postgres:
// a reserve and a contribute race on the SAME item. The shared item lock serializes
// them, so exactly one commits its track and the other gets ErrCrossTrackConflict —
// never both, no interleaving that lets both start.
func TestPostgres_ReserveCoBuyMutualExclusionRace(t *testing.T) {
	ctx := context.Background()
	st := newPostgresStore(t)
	suffix := t.Name()

	ten, err := st.CreateTenant(ctx, storage.Tenant{Subdomain: "mx-" + suffix})
	require.NoError(t, err)
	s := st.ForTenant(ten)
	owner, err := s.Users().Create(ctx, storage.User{Name: "Owner"})
	require.NoError(t, err)
	list, err := s.Lists().Create(ctx, storage.List{Title: "Mutual"}, owner.ID)
	require.NoError(t, err)

	for round := 0; round < 40; round++ {
		// A fresh priced item each round so both tracks start from a clean slate.
		item, err := s.Items().Create(ctx, storage.Item{
			ListID: list.ID, Name: "Gift", QuantityWanted: 1,
			Price: &storage.Money{AmountMinor: 10000, Currency: "EUR"},
		})
		require.NoError(t, err)

		var (
			wg              sync.WaitGroup
			mu              sync.Mutex
			reserved, cobuy int
			start           = make(chan struct{})
		)
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			_, err := s.Reservations().CreateWithinCapacity(ctx, storage.Reservation{
				ItemID: item.ID, Quantity: 1, TokenHash: fmt.Sprintf("res-%d", round),
				State: storage.StateActive,
			}, item.QuantityWanted)
			if err == nil {
				mu.Lock()
				reserved++
				mu.Unlock()
			} else {
				assert.ErrorIs(t, err, storage.ErrCrossTrackConflict)
			}
		}()
		go func() {
			defer wg.Done()
			<-start
			_, err := s.Contributions().CreateWithinCapacity(ctx, storage.Contribution{
				ItemID: item.ID, Pledged: storage.Money{AmountMinor: 10000, Currency: "EUR"},
				ContactEmail: "a@example.com", TokenHash: fmt.Sprintf("con-%d", round),
			}, item.Price.AmountMinor)
			if err == nil {
				mu.Lock()
				cobuy++
				mu.Unlock()
			} else {
				assert.ErrorIs(t, err, storage.ErrCrossTrackConflict)
			}
		}()
		close(start)
		wg.Wait()

		require.Equalf(t, 1, reserved+cobuy,
			"exactly one track wins the item under real concurrency (round %d: reserved=%d cobuy=%d)",
			round, reserved, cobuy)
	}
}

// TestPostgres_MatchExpiryVsConfirmRace races the co-buy auto-expiry sweep (#101)
// against a full confirmation of the same proposed match on real Postgres. Both the
// expiry and every confirm run under the item row lock, so the two transitions
// serialize: the match must land in exactly one consistent terminal shape — either
// both_confirmed with every pledge confirmed, or expired with every pledge expired —
// never a torn mix. 40 rounds, each on a fresh match.
func TestPostgres_MatchExpiryVsConfirmRace(t *testing.T) {
	ctx := context.Background()
	st := newPostgresStore(t)
	suffix := t.Name()

	ten, err := st.CreateTenant(ctx, storage.Tenant{Subdomain: "mex-" + suffix})
	require.NoError(t, err)
	s := st.ForTenant(ten)
	owner, err := s.Users().Create(ctx, storage.User{Name: "Owner"})
	require.NoError(t, err)
	list, err := s.Lists().Create(ctx, storage.List{Title: "Expiry"}, owner.ID)
	require.NoError(t, err)

	for round := 0; round < 40; round++ {
		item, err := s.Items().Create(ctx, storage.Item{
			ListID: list.ID, Name: "Gift", QuantityWanted: 1,
			Price: &storage.Money{AmountMinor: 10000, Currency: "EUR"},
		})
		require.NoError(t, err)

		// Two pledges + a proposed match linking them (the store-level shape).
		var ids []string
		for i := 0; i < 2; i++ {
			c, err := s.Contributions().Create(ctx, storage.Contribution{
				ItemID: item.ID, Pledged: storage.Money{AmountMinor: 5000, Currency: "EUR"},
				ContactEmail: fmt.Sprintf("p%d-%d@example.com", i, round),
				TokenHash:    fmt.Sprintf("cap-%d-%d", i, round),
			})
			require.NoError(t, err)
			ids = append(ids, c.ID)
		}
		m, err := s.Matches().Create(ctx, storage.Match{ItemID: item.ID, ContributionIDs: ids})
		require.NoError(t, err)

		var (
			wg    sync.WaitGroup
			start = make(chan struct{})
		)
		wg.Add(2)
		// Sweep: expire the still-proposed match.
		go func() {
			defer wg.Done()
			<-start
			_, err := s.Matches().ExpireIfProposed(ctx, item.ID, m.ID, time.Now().UTC())
			assert.NoError(t, err)
		}()
		// Confirmation: both parties confirm, completing the match if it wins the race.
		go func() {
			defer wg.Done()
			<-start
			for _, id := range ids {
				if _, _, _, err := s.Matches().ConfirmContribution(ctx, item.ID, m.ID, id); err != nil {
					assert.NoError(t, err)
				}
			}
		}()
		close(start)
		wg.Wait()

		// Whichever won, the result is internally consistent — no torn state.
		final, err := s.Matches().Get(ctx, m.ID)
		require.NoError(t, err)
		require.Contains(t, []storage.MatchState{storage.MatchBothConfirmed, storage.MatchExpired},
			final.State, "round %d: match must be a single terminal state", round)

		want := storage.ContributionConfirmed
		if final.State == storage.MatchExpired {
			want = storage.ContributionExpired
		}
		for _, id := range ids {
			c, err := s.Contributions().Get(ctx, id)
			require.NoError(t, err)
			require.Equalf(t, want, c.Status,
				"round %d: match %s → every pledge must be %s (got %s)", round, final.State, want, c.Status)
		}
	}
}
