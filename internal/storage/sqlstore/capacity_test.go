package sqlstore_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/storage"
)

func TestCreateWithinCapacity(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ts, _, list := seedList(t, st)
	item, err := ts.Items().Create(ctx, storage.Item{ListID: list.ID, Name: "Seats", QuantityWanted: 3})
	require.NoError(t, err)

	// Three single-unit reservations fit exactly.
	for i := 0; i < 3; i++ {
		_, err := ts.Reservations().CreateWithinCapacity(ctx, storage.Reservation{
			ItemID: item.ID, Quantity: 1, TokenHash: fmt.Sprintf("t%d", i),
		}, item.QuantityWanted)
		require.NoError(t, err)
	}

	// The fourth exceeds the wanted quantity.
	_, err = ts.Reservations().CreateWithinCapacity(ctx, storage.Reservation{
		ItemID: item.ID, Quantity: 1, TokenHash: "t3",
	}, item.QuantityWanted)
	assert.ErrorIs(t, err, storage.ErrCapacityExceeded)

	// A single over-cap reservation is rejected up front.
	item2, err := ts.Items().Create(ctx, storage.Item{ListID: list.ID, Name: "Pair", QuantityWanted: 2})
	require.NoError(t, err)
	_, err = ts.Reservations().CreateWithinCapacity(ctx, storage.Reservation{
		ItemID: item2.ID, Quantity: 5, TokenHash: "big",
	}, item2.QuantityWanted)
	assert.ErrorIs(t, err, storage.ErrCapacityExceeded)

	// Unknown item.
	_, err = ts.Reservations().CreateWithinCapacity(ctx, storage.Reservation{
		ItemID: "nope", Quantity: 1, TokenHash: "z",
	}, 5)
	assert.ErrorIs(t, err, storage.ErrNotFound)

	// Exactly 3 landed.
	total, err := ts.Items().ReservedQuantity(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, 3, total)
}

// TestContributeWithinCapacity mirrors the reservation guard for contributions:
// pledges accumulate up to the item price, and one that would overfund is
// rejected atomically.
func TestContributeWithinCapacity(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ts, _, list := seedList(t, st)
	item, err := ts.Items().Create(ctx, storage.Item{ListID: list.ID, Name: "Machine"})
	require.NoError(t, err)
	const price = int64(40000)

	mk := func(minor int64, tok string) error {
		_, err := ts.Contributions().CreateWithinCapacity(ctx, storage.Contribution{
			ItemID:       item.ID,
			Pledged:      storage.Money{AmountMinor: minor, Currency: "EUR"},
			ContactEmail: tok + "@example.com",
			TokenHash:    tok,
		}, price)
		return err
	}

	require.NoError(t, mk(20000, "c1"))
	require.NoError(t, mk(20000, "c2")) // exactly covers
	assert.ErrorIs(t, mk(1, "c3"), storage.ErrCapacityExceeded)

	funded, err := ts.Items().FundedAmount(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, price, funded.AmountMinor)
}

// TestCreateWithinCapacityConcurrent is the anti-oversell proof: many goroutines
// race to reserve a limited item; the atomic guard admits exactly the wanted
// quantity and rejects the rest with ErrCapacityExceeded — no oversell.
func TestCreateWithinCapacityConcurrent(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ts, _, list := seedList(t, st)
	const wanted = 5
	item, err := ts.Items().Create(ctx, storage.Item{ListID: list.ID, Name: "Limited", QuantityWanted: wanted})
	require.NoError(t, err)

	const n = 20
	results := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := ts.Reservations().CreateWithinCapacity(ctx, storage.Reservation{
				ItemID: item.ID, Quantity: 1, TokenHash: fmt.Sprintf("c%d", i),
			}, wanted)
			results[i] = err
		}(i)
	}
	wg.Wait()

	success, rejected := 0, 0
	for _, e := range results {
		switch {
		case e == nil:
			success++
		case errors.Is(e, storage.ErrCapacityExceeded):
			rejected++
		default:
			t.Fatalf("unexpected error: %v", e)
		}
	}
	assert.Equal(t, wanted, success, "exactly the wanted quantity is admitted")
	assert.Equal(t, n-wanted, rejected)

	total, err := ts.Items().ReservedQuantity(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, wanted, total, "no oversell")
}
