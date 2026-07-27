package sqlstore_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/storage"
)

func strp(s string) *string { return &s }

// seedList returns a tenant handle plus an owner and one list to hang items off.
func seedList(t *testing.T, st storage.Store) (storage.TenantStore, storage.User, storage.List) {
	t.Helper()
	ctx := context.Background()
	ten := mkTenant(t, st, "alice")
	ts := st.ForTenant(ten)
	owner, err := ts.Users().Create(ctx, storage.User{Name: "Alice"})
	require.NoError(t, err)
	list, err := ts.Lists().Create(ctx, storage.List{Title: "Birthday"}, owner.ID)
	require.NoError(t, err)
	return ts, owner, list
}

func TestListCRUD(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ts, owner, _ := seedList(t, st)

	event := time.Date(2027, 3, 14, 0, 0, 0, 0, time.UTC)
	created, err := ts.Lists().Create(ctx, storage.List{
		Title:      "Wedding",
		Visibility: storage.VisibilityUnlisted,
		EventDate:  &event,
		DecayDays:  iptr(14),
		Active:     true,
	}, owner.ID)
	require.NoError(t, err)
	assert.NotEmpty(t, created.ID)
	assert.NotEmpty(t, created.ShareSlug, "slug auto-generated when empty")

	got, err := ts.Lists().Get(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, "Wedding", got.Title)
	assert.Equal(t, storage.VisibilityUnlisted, got.Visibility)
	require.NotNil(t, got.EventDate)
	assert.Equal(t, "2027-03-14", got.EventDate.Format("2006-01-02"))
	require.NotNil(t, got.DecayDays)
	assert.Equal(t, 14, *got.DecayDays)
	assert.True(t, got.Active)

	// Resolve by slug (public surface path).
	bySlug, err := ts.Lists().GetBySlug(ctx, created.ShareSlug)
	require.NoError(t, err)
	assert.Equal(t, created.ID, bySlug.ID)

	// Update: clear the event date, flip active.
	got.Title = "Wedding (final)"
	got.EventDate = nil
	got.Active = false
	updated, err := ts.Lists().Update(ctx, got)
	require.NoError(t, err)
	assert.Equal(t, "Wedding (final)", updated.Title)
	assert.Nil(t, updated.EventDate)
	assert.False(t, updated.Active)

	// Two lists for this owner now (seed + Wedding); list is paginated.
	page, total, err := ts.Lists().List(ctx, owner.ID, storage.Page{Limit: 1, Offset: 0})
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.Len(t, page, 1)

	require.NoError(t, ts.Lists().Delete(ctx, created.ID))
	_, err = ts.Lists().Get(ctx, created.ID)
	assert.ErrorIs(t, err, storage.ErrNotFound)
}

func TestItemCRUDAndNullables(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ts, _, list := seedList(t, st)

	// Full item with a price and optional fields.
	full, err := ts.Items().Create(ctx, storage.Item{
		ListID:         list.ID,
		Name:           "Headphones",
		URL:            strp("https://shop.example/hp"),
		ImageURL:       strp("https://shop.example/hp.jpg"),
		Price:          &storage.Money{AmountMinor: 19900, Currency: "EUR"},
		Note:           strp("the over-ear ones"),
		Priority:       5,
		QuantityWanted: 2,
	})
	require.NoError(t, err)

	got, err := ts.Items().Get(ctx, full.ID)
	require.NoError(t, err)
	require.NotNil(t, got.Price)
	assert.Equal(t, int64(19900), got.Price.AmountMinor)
	assert.Equal(t, "EUR", got.Price.Currency)
	assert.Equal(t, strp("the over-ear ones"), got.Note)
	assert.Equal(t, 2, got.QuantityWanted)

	// Minimal item: no price, no optionals; quantity defaults to 1.
	bare, err := ts.Items().Create(ctx, storage.Item{ListID: list.ID, Name: "Socks"})
	require.NoError(t, err)
	gotBare, err := ts.Items().Get(ctx, bare.ID)
	require.NoError(t, err)
	assert.Nil(t, gotBare.Price)
	assert.Nil(t, gotBare.URL)
	assert.Equal(t, 1, gotBare.QuantityWanted)

	// Update clears the price.
	got.Price = nil
	got.Name = "Headphones v2"
	updated, err := ts.Items().Update(ctx, got)
	require.NoError(t, err)
	assert.Nil(t, updated.Price)
	assert.Equal(t, "Headphones v2", updated.Name)

	items, total, err := ts.Items().ListByList(ctx, list.ID, storage.Page{Limit: 100})
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.Len(t, items, 2)
}

func TestReservationsAndReservedQuantity(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ts, _, list := seedList(t, st)
	item, err := ts.Items().Create(ctx, storage.Item{ListID: list.ID, Name: "Blender", QuantityWanted: 3})
	require.NoError(t, err)

	r1, err := ts.Reservations().Create(ctx, storage.Reservation{
		ItemID:    item.ID,
		GiverName: strp("Sam"),
		Quantity:  2,
		TokenHash: "hash-1",
	})
	require.NoError(t, err)
	_, err = ts.Reservations().Create(ctx, storage.Reservation{
		ItemID:    item.ID,
		Quantity:  1,
		TokenHash: "hash-2",
	})
	require.NoError(t, err)

	// Duplicate token hash conflicts.
	_, err = ts.Reservations().Create(ctx, storage.Reservation{ItemID: item.ID, TokenHash: "hash-1"})
	assert.ErrorIs(t, err, storage.ErrConflict)

	byTok, err := ts.Reservations().ByTokenHash(ctx, "hash-1")
	require.NoError(t, err)
	assert.Equal(t, r1.ID, byTok.ID)
	assert.Equal(t, strp("Sam"), byTok.GiverName)

	qty, err := ts.Items().ReservedQuantity(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, 3, qty)

	require.NoError(t, ts.Reservations().Delete(ctx, r1.ID))
	qty, err = ts.Items().ReservedQuantity(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, qty)
}

// TestCoBuyingHandshake exercises the contribution → match → confirm flow that
// backs ADR-0002 §6, at the storage level.
func TestCoBuyingHandshake(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ts, _, list := seedList(t, st)
	item, err := ts.Items().Create(ctx, storage.Item{
		ListID: list.ID,
		Name:   "Espresso machine",
		Price:  &storage.Money{AmountMinor: 40000, Currency: "EUR"},
	})
	require.NoError(t, err)

	c1, err := ts.Contributions().Create(ctx, storage.Contribution{
		ItemID:       item.ID,
		Pledged:      storage.Money{AmountMinor: 20000, Currency: "EUR"},
		ContactEmail: "one@example.com",
		TokenHash:    "ctok-1",
	})
	require.NoError(t, err)
	assert.Equal(t, storage.ContributionPending, c1.Status)

	c2, err := ts.Contributions().Create(ctx, storage.Contribution{
		ItemID:       item.ID,
		Pledged:      storage.Money{AmountMinor: 20000, Currency: "EUR"},
		ContactEmail: "two@example.com",
		TokenHash:    "ctok-2",
	})
	require.NoError(t, err)

	// Funded amount sums the pending pledges.
	funded, err := ts.Items().FundedAmount(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(40000), funded.AmountMinor)
	assert.Equal(t, "EUR", funded.Currency)

	// Pledges complete funding → propose a match linking both.
	match, err := ts.Matches().Create(ctx, storage.Match{
		ItemID:          item.ID,
		ContributionIDs: []string{c1.ID, c2.ID},
	})
	require.NoError(t, err)
	assert.Equal(t, storage.MatchProposed, match.State)
	assert.ElementsMatch(t, []string{c1.ID, c2.ID}, match.ContributionIDs)

	// Creating the match stamped both contributions.
	got1, err := ts.Contributions().Get(ctx, c1.ID)
	require.NoError(t, err)
	require.NotNil(t, got1.MatchID)
	assert.Equal(t, match.ID, *got1.MatchID)
	assert.Equal(t, storage.ContributionMatched, got1.Status)

	// The first pledger can discover the match via ByTokenHash → match_id.
	viaToken, err := ts.Contributions().ByTokenHash(ctx, "ctok-1")
	require.NoError(t, err)
	require.NotNil(t, viaToken.MatchID)
	assert.Equal(t, match.ID, *viaToken.MatchID)

	// Both confirm → state advances.
	match.State = storage.MatchBothConfirmed
	updated, err := ts.Matches().Update(ctx, match)
	require.NoError(t, err)
	assert.Equal(t, storage.MatchBothConfirmed, updated.State)

	byItem, err := ts.Matches().ListByItem(ctx, item.ID)
	require.NoError(t, err)
	require.Len(t, byItem, 1)
	assert.Len(t, byItem[0].ContributionIDs, 2)
}

func TestContributionWithdraw(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ts, _, list := seedList(t, st)
	item, err := ts.Items().Create(ctx, storage.Item{ListID: list.ID, Name: "Tent"})
	require.NoError(t, err)

	c, err := ts.Contributions().Create(ctx, storage.Contribution{
		ItemID:       item.ID,
		Pledged:      storage.Money{AmountMinor: 5000, Currency: "USD"},
		ContactEmail: "giver@example.com",
		TokenHash:    "wtok",
	})
	require.NoError(t, err)

	require.NoError(t, ts.Contributions().Delete(ctx, c.ID))
	_, err = ts.Contributions().Get(ctx, c.ID)
	assert.ErrorIs(t, err, storage.ErrNotFound)

	funded, err := ts.Items().FundedAmount(ctx, item.ID)
	require.NoError(t, err)
	assert.Zero(t, funded.AmountMinor)
}

func TestDomainCRUD(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ten := mkTenant(t, st, "alice")
	ts := st.ForTenant(ten)

	d, err := ts.Domains().Create(ctx, storage.Domain{
		Hostname:    "gifts.example.com",
		CNAMETarget: "alias.host.example",
	})
	require.NoError(t, err)
	assert.Equal(t, storage.TLSNone, d.TLSStatus)
	assert.False(t, d.Verified)

	// Hostname is globally unique.
	tenB := mkTenant(t, st, "bob")
	_, err = st.ForTenant(tenB).Domains().Create(ctx, storage.Domain{
		Hostname:    "gifts.example.com",
		CNAMETarget: "other.host.example",
	})
	assert.ErrorIs(t, err, storage.ErrConflict)

	d.Verified = true
	d.TLSStatus = storage.TLSActive
	_, err = ts.Domains().Update(ctx, d)
	require.NoError(t, err)

	all, err := ts.Domains().List(ctx)
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.True(t, all[0].Verified)
	assert.Equal(t, storage.TLSActive, all[0].TLSStatus)
}
