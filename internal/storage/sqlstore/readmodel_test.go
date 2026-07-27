package sqlstore_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/storage"
)

// TestListItemCount checks the derived item_count on list reads (populated by
// Get/GetBySlug/List, zero on Create).
func TestListItemCount(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ts, owner, list := seedList(t, st)

	// Fresh Create reports zero (not re-read).
	assert.Zero(t, list.ItemCount)

	for _, n := range []string{"a", "b", "c"} {
		_, err := ts.Items().Create(ctx, storage.Item{ListID: list.ID, Name: n})
		require.NoError(t, err)
	}

	got, err := ts.Lists().Get(ctx, list.ID)
	require.NoError(t, err)
	assert.Equal(t, 3, got.ItemCount)

	bySlug, err := ts.Lists().GetBySlug(ctx, list.ShareSlug)
	require.NoError(t, err)
	assert.Equal(t, 3, bySlug.ItemCount)

	page, _, err := ts.Lists().List(ctx, owner.ID, storage.Page{Limit: 10})
	require.NoError(t, err)
	require.NotEmpty(t, page)
	for _, l := range page {
		if l.ID == list.ID {
			assert.Equal(t, 3, l.ItemCount)
		}
	}
}

// TestBatchAggregates checks the N+1-avoiding batch reservation/funding helpers
// return per-item totals across a whole list, tenant-scoped.
func TestBatchAggregates(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ts, _, list := seedList(t, st)

	i1, err := ts.Items().Create(ctx, storage.Item{ListID: list.ID, Name: "one", QuantityWanted: 5})
	require.NoError(t, err)
	i2, err := ts.Items().Create(ctx, storage.Item{ListID: list.ID, Name: "two"})
	require.NoError(t, err)
	// i3 has nothing on it — must be absent from both maps.
	i3, err := ts.Items().Create(ctx, storage.Item{ListID: list.ID, Name: "three"})
	require.NoError(t, err)

	_, err = ts.Reservations().Create(ctx, storage.Reservation{ItemID: i1.ID, Quantity: 2, TokenHash: "t1"})
	require.NoError(t, err)
	_, err = ts.Reservations().Create(ctx, storage.Reservation{ItemID: i1.ID, Quantity: 1, TokenHash: "t2"})
	require.NoError(t, err)
	_, err = ts.Contributions().Create(ctx, storage.Contribution{
		ItemID: i2.ID, Pledged: storage.Money{AmountMinor: 1500, Currency: "EUR"},
		ContactEmail: "g@example.com", TokenHash: "c1",
	})
	require.NoError(t, err)

	reserved, err := ts.Items().ReservedQuantitiesByList(ctx, list.ID)
	require.NoError(t, err)
	assert.Equal(t, 3, reserved[i1.ID])
	_, hasI2 := reserved[i2.ID]
	assert.False(t, hasI2)
	_, hasI3 := reserved[i3.ID]
	assert.False(t, hasI3)

	funded, err := ts.Items().FundedAmountsByList(ctx, list.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1500), funded[i2.ID].AmountMinor)
	assert.Equal(t, "EUR", funded[i2.ID].Currency)
	_, hasI1 := funded[i1.ID]
	assert.False(t, hasI1)
}

// TestBatchAggregatesTenantScoped confirms the batch helpers never cross tenants.
func TestBatchAggregatesTenantScoped(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	tenA := mkTenant(t, st, "alice")
	tenB := mkTenant(t, st, "bob")
	as := st.ForTenant(tenA)
	bs := st.ForTenant(tenB)

	ownerA, err := as.Users().Create(ctx, storage.User{Name: "A"})
	require.NoError(t, err)
	listA, err := as.Lists().Create(ctx, storage.List{Title: "A"}, ownerA.ID)
	require.NoError(t, err)
	itemA, err := as.Items().Create(ctx, storage.Item{ListID: listA.ID, Name: "x"})
	require.NoError(t, err)
	_, err = as.Reservations().Create(ctx, storage.Reservation{ItemID: itemA.ID, Quantity: 4, TokenHash: "ta"})
	require.NoError(t, err)

	// B querying A's list id sees nothing (scoped by B's tenant).
	reserved, err := bs.Items().ReservedQuantitiesByList(ctx, listA.ID)
	require.NoError(t, err)
	assert.Empty(t, reserved)
}
