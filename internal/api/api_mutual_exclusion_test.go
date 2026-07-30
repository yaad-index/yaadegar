package api_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/api/gen"
)

// #93: reserve and co-buy are mutually exclusive per item, regardless of quantity.

func TestReserveThenContribute409(t *testing.T) {
	h := newHarness(t)
	list := h.createList("L")
	item := h.pricedItem(*list.Id, 10000, "EUR")

	resp, _ := h.reserve(*list.ShareSlug, *item.Id, 1)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	resp, _ = h.pledge(*list.ShareSlug, *item.Id, 5000, "EUR", "a@example.com")
	assert.Equal(t, http.StatusConflict, resp.StatusCode, "can't co-buy a reserved item")
}

func TestContributeThenReserve409(t *testing.T) {
	h := newHarness(t)
	list := h.createList("L")
	item := h.pricedItem(*list.Id, 10000, "EUR")

	resp, _ := h.pledge(*list.ShareSlug, *item.Id, 5000, "EUR", "a@example.com")
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	resp, _ = h.reserve(*list.ShareSlug, *item.Id, 1)
	assert.Equal(t, http.StatusConflict, resp.StatusCode, "can't reserve an item being co-bought")
}

// Even a partial reservation (1 of many units) takes the whole item — per-item
// exclusion regardless of quantity (ADR-0002 amendment).
func TestPartialReserveBlocksCoBuy(t *testing.T) {
	h := newHarness(t)
	list := h.createList("L")
	item := h.pricedItem(*list.Id, 10000, "EUR")
	item = h.setItemQuantity(t, *item.Id, 3)

	resp, _ := h.reserve(*list.ShareSlug, *item.Id, 1) // 1 of 3 units
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp, _ = h.pledge(*list.ShareSlug, *item.Id, 5000, "EUR", "a@example.com")
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
}

func TestReleaseFreesForCoBuy(t *testing.T) {
	h := newHarness(t)
	list := h.createList("L")
	item := h.pricedItem(*list.Id, 10000, "EUR")

	resp, res := h.reserve(*list.ShareSlug, *item.Id, 1)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	// Release the reservation.
	resp, _ = h.reqH(http.MethodDelete, "/public/reservations/"+res.ReservationId, h.ownerHost(),
		map[string]string{"X-Capability-Token": *res.CapabilityToken}, nil)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	// The item is free for co-buy again.
	resp, _ = h.pledge(*list.ShareSlug, *item.Id, 5000, "EUR", "a@example.com")
	assert.Equal(t, http.StatusCreated, resp.StatusCode)
}

// A reservation that has decayed to expired no longer holds the item, so co-buy is
// allowed again (the exclusion counts state != expired).
func TestDecayExpiryFreesForCoBuy(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	list := h.createList("L")
	item := h.pricedItem(*list.Id, 10000, "EUR")

	resp, res := h.reserve(*list.ShareSlug, *item.Id, 1)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	// Drive it through the decay states to expired (active → notified → expired).
	rs := h.store.ForTenant(h.tenant).Reservations()
	now := h.clk.Now()
	_, err := rs.MarkReserverNotified(ctx, res.ReservationId, now, "keep", "rel")
	require.NoError(t, err)
	_, err = rs.MarkExpired(ctx, res.ReservationId, now)
	require.NoError(t, err)

	resp, _ = h.pledge(*list.ShareSlug, *item.Id, 5000, "EUR", "a@example.com")
	assert.Equal(t, http.StatusCreated, resp.StatusCode, "an expired reservation frees the item for co-buy")
}

// TestDissolvedMatchDoesNotFreeForReserve: a declined match resets the other
// participants to pending (still live), so the item stays blocked for reserve until
// those pledges are withdrawn.
func TestDissolvedMatchDoesNotFreeForReserve(t *testing.T) {
	h := newHarness(t)
	list := h.createList("L")
	item := h.pricedItem(*list.Id, 10000, "EUR")
	_, a := h.pledge(*list.ShareSlug, *item.Id, 5000, "EUR", "a@example.com")
	_, b := h.pledge(*list.ShareSlug, *item.Id, 5000, "EUR", "b@example.com")
	require.NotNil(t, b.Match)

	// A declines → match dissolves, B is reset to pending (still a live co-buy).
	resp, _ := h.confirm(*b.Match.Id, *a.CapabilityToken, "decline")
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp, _ = h.reserve(*list.ShareSlug, *item.Id, 1)
	assert.Equal(t, http.StatusConflict, resp.StatusCode,
		"a dissolved match still blocks reserve while a pledge stays pending")
	_ = a
}

// TestAllPledgesWithdrawnFreesForReserve: once every contribution is terminal
// (declined/withdrawn), the item frees for reserve.
func TestAllPledgesWithdrawnFreesForReserve(t *testing.T) {
	h := newHarness(t)
	list := h.createList("L")
	item := h.pricedItem(*list.Id, 10000, "EUR")
	_, a := h.pledge(*list.ShareSlug, *item.Id, 5000, "EUR", "a@example.com")
	_, b := h.pledge(*list.ShareSlug, *item.Id, 5000, "EUR", "b@example.com")
	require.NotNil(t, b.Match)

	// A declines (A → declined), leaving B pending.
	resp, _ := h.confirm(*b.Match.Id, *a.CapabilityToken, "decline")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	// Withdraw B → no live contribution remains.
	resp, _ = h.reqH(http.MethodDelete, "/public/contributions/"+*b.ContributionId, h.ownerHost(),
		map[string]string{"X-Capability-Token": *b.CapabilityToken}, nil)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	resp, _ = h.reserve(*list.ShareSlug, *item.Id, 1)
	assert.Equal(t, http.StatusCreated, resp.StatusCode,
		"with every pledge terminal, the item frees for reserve")
}

// setItemQuantity is a small owner-side helper to bump quantity_wanted for the
// per-item-regardless-of-quantity test.
func (h *harness) setItemQuantity(t *testing.T, itemID string, qty int) gen.Item {
	t.Helper()
	resp, body := h.req(http.MethodPatch, "/api/v1/items/"+itemID, h.ownerHost(), h.ownerToken(),
		map[string]any{"quantity_wanted": qty})
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	return decode[gen.Item](t, body)
}
