package api_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/api/gen"
)

// reserve is a helper: reserve qty units of an item as an anonymous giver.
func (h *harness) reserve(slug, itemID string, qty int) (*http.Response, gen.ReservationCreated) {
	h.t.Helper()
	resp, body := h.req(http.MethodPost, "/public/"+slug+"/items/"+itemID+"/reservations",
		h.ownerHost(), "", map[string]any{"quantity": qty})
	if resp.StatusCode != http.StatusCreated {
		return resp, gen.ReservationCreated{}
	}
	return resp, decode[gen.ReservationCreated](h.t, body)
}

func TestReservationReleaseRoundTrip(t *testing.T) {
	h := newHarness(t)
	list := h.createList("List")
	item := h.createItem(*list.Id, "Item", 1)

	resp, created := h.reserve(*list.ShareSlug, *item.Id, 1)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	token := *created.CapabilityToken

	// Now reserved.
	assert.Equal(t, gen.Reserved, h.itemAvailability(t, *list.Id, *item.Id))

	// Release with the capability token → 204, back to available.
	resp, _ = h.reqH(http.MethodDelete, "/public/reservations/"+created.ReservationId,
		h.ownerHost(), map[string]string{"X-Capability-Token": token}, nil)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Equal(t, gen.Available, h.itemAvailability(t, *list.Id, *item.Id))
}

func TestReservationReleaseAuth(t *testing.T) {
	h := newHarness(t)
	list := h.createList("List")
	item := h.createItem(*list.Id, "Item", 1)
	_, created := h.reserve(*list.ShareSlug, *item.Id, 1)

	// Missing token → 401.
	resp, _ := h.reqH(http.MethodDelete, "/public/reservations/"+created.ReservationId,
		h.ownerHost(), nil, nil)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	// Wrong token → 401.
	resp, _ = h.reqH(http.MethodDelete, "/public/reservations/"+created.ReservationId,
		h.ownerHost(), map[string]string{"X-Capability-Token": "wrong-token"}, nil)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	// Valid token but wrong reservation id → 404.
	resp, _ = h.reqH(http.MethodDelete, "/public/reservations/does-not-exist",
		h.ownerHost(), map[string]string{"X-Capability-Token": *created.CapabilityToken}, nil)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	// Still reserved (nothing was released).
	assert.Equal(t, gen.Reserved, h.itemAvailability(t, *list.Id, *item.Id))
}

func TestReservationFullyReserved409(t *testing.T) {
	h := newHarness(t)
	list := h.createList("List")
	item := h.createItem(*list.Id, "Single", 1)

	resp, _ := h.reserve(*list.ShareSlug, *item.Id, 1)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	// A second unit exceeds quantity_wanted (1) → 409.
	resp, _ = h.reserve(*list.ShareSlug, *item.Id, 1)
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
}

// TestUpdateItemQuantityClamped guards an edge case: a PATCH to
// quantity_wanted=0 must not make an item permanently unreservable. Storage
// clamps it to 1 (consistent with Create), so the item stays reservable.
func TestUpdateItemQuantityClamped(t *testing.T) {
	h := newHarness(t)
	list := h.createList("List")
	item := h.createItem(*list.Id, "Item", 3)

	resp, body := h.req(http.MethodPatch, "/api/v1/items/"+*item.Id, h.ownerHost(), h.ownerToken(),
		map[string]any{"quantity_wanted": 0})
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	assert.Equal(t, 1, *decode[gen.Item](t, body).QuantityWanted)

	// Still reservable.
	resp, _ = h.reserve(*list.ShareSlug, *item.Id, 1)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)
}

func TestReservationUnknownTargets404(t *testing.T) {
	h := newHarness(t)
	list := h.createList("List")
	item := h.createItem(*list.Id, "Item", 1)

	// Unknown slug.
	resp, _ := h.reserve("no-such-slug", *item.Id, 1)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	// Unknown item on a real list.
	resp, _ = h.reserve(*list.ShareSlug, "no-such-item", 1)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// itemAvailability reads an item's current availability via the owner list view.
func (h *harness) itemAvailability(t *testing.T, listID, itemID string) gen.ItemAvailability {
	t.Helper()
	resp, body := h.req(http.MethodGet, "/api/v1/lists/"+listID+"/items", h.ownerHost(), h.ownerToken(), nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	for _, it := range *decode[gen.ItemPage](t, body).Items {
		if *it.Id == itemID {
			return *it.Availability
		}
	}
	t.Fatalf("item %s not found", itemID)
	return ""
}
