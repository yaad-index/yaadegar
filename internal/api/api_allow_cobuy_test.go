package api_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/api/gen"
)

// #100: owner opt-out of group-buy. A per-item allow_cobuy override resolves over a
// per-list default; when co-buy is off the contribute endpoint returns 403 and the
// item is reserve-only.

// setItemAllowCobuy patches the per-item override (owner surface).
func (h *harness) setItemAllowCobuy(t *testing.T, itemID string, allow bool) gen.Item {
	t.Helper()
	resp, body := h.req(http.MethodPatch, "/api/v1/items/"+itemID, h.ownerHost(), h.ownerToken(),
		map[string]any{"allow_cobuy": allow})
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	return decode[gen.Item](t, body)
}

// setListAllowCobuy patches the list-level co-buy default (owner surface).
func (h *harness) setListAllowCobuy(t *testing.T, listID string, allow bool) gen.List {
	t.Helper()
	resp, body := h.req(http.MethodPatch, "/api/v1/lists/"+listID, h.ownerHost(), h.ownerToken(),
		map[string]any{"allow_cobuy": allow})
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	return decode[gen.List](t, body)
}

// publicItem fetches a single item from the giver-facing public list view.
func (h *harness) publicItem(t *testing.T, slug, itemID string) gen.PublicItem {
	t.Helper()
	resp, body := h.req(http.MethodGet, "/public/"+slug, h.ownerHost(), "", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	pl := decode[gen.PublicList](t, body)
	require.NotNil(t, pl.Items)
	for _, it := range *pl.Items {
		if it.Id != nil && *it.Id == itemID {
			return it
		}
	}
	t.Fatalf("item %s not found in public list", itemID)
	return gen.PublicItem{}
}

// A new list defaults to co-buy-enabled and items inherit it, so a pledge succeeds.
func TestAllowCobuy_DefaultsEnabled(t *testing.T) {
	h := newHarness(t)
	list := h.createList("L")
	item := h.pricedItem(*list.Id, 10000, "EUR")

	require.NotNil(t, list.AllowCobuy)
	assert.True(t, *list.AllowCobuy, "a new list defaults to co-buy enabled")

	resp, _ := h.pledge(*list.ShareSlug, *item.Id, 5000, "EUR", "a@example.com")
	assert.Equal(t, http.StatusCreated, resp.StatusCode)
	assert.True(t, *h.publicItem(t, *list.ShareSlug, *item.Id).AllowCobuy)
}

// Turning off the per-item override makes the item reserve-only: contribute → 403,
// the public item reports allow_cobuy=false, and reserve still works.
func TestAllowCobuy_ItemOverrideOff(t *testing.T) {
	h := newHarness(t)
	list := h.createList("L")
	item := h.pricedItem(*list.Id, 10000, "EUR")
	h.setItemAllowCobuy(t, *item.Id, false)

	resp, body := h.pledge(*list.ShareSlug, *item.Id, 5000, "EUR", "a@example.com")
	assert.Equal(t, http.StatusForbidden, resp.StatusCode, "co-buying is off → 403: %s", body)

	assert.False(t, *h.publicItem(t, *list.ShareSlug, *item.Id).AllowCobuy)

	// Reserve is unaffected — the item is reserve-only.
	resp, _ = h.reserve(*list.ShareSlug, *item.Id, 1)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)
}

// The per-item override beats the list default in both directions.
func TestAllowCobuy_ItemOverridesListDefault(t *testing.T) {
	h := newHarness(t)
	list := h.createList("L")

	// List default OFF, but an item overrides it ON → pledge succeeds.
	h.setListAllowCobuy(t, *list.Id, false)
	on := h.pricedItem(*list.Id, 10000, "EUR")
	h.setItemAllowCobuy(t, *on.Id, true)
	resp, _ := h.pledge(*list.ShareSlug, *on.Id, 5000, "EUR", "a@example.com")
	assert.Equal(t, http.StatusCreated, resp.StatusCode, "item override ON beats list default OFF")

	// A sibling item with no override inherits the list default OFF → 403.
	inherit := h.pricedItem(*list.Id, 10000, "EUR")
	resp, _ = h.pledge(*list.ShareSlug, *inherit.Id, 5000, "EUR", "b@example.com")
	assert.Equal(t, http.StatusForbidden, resp.StatusCode, "no override inherits list default OFF")
}

// A disallowed-from-creation item never derives co_buying (it can hold no pledges).
func TestAllowCobuy_DisallowedItemNeverCoBuying(t *testing.T) {
	h := newHarness(t)
	list := h.createList("L")
	item := h.pricedItem(*list.Id, 10000, "EUR")
	h.setItemAllowCobuy(t, *item.Id, false)

	pub := h.publicItem(t, *list.ShareSlug, *item.Id)
	assert.Equal(t, gen.ItemAvailability("available"), *pub.Availability)
}

// TestAllowCobuy_GrandfatherWindow: disabling co-buy on an item that already has a
// live pledge does NOT retroactively free it — the pledge is grandfathered, so
// reserve stays blocked by the #93 cross-track check until the pledge is withdrawn,
// after which reserve succeeds. Pins the bounded transient explicitly.
func TestAllowCobuy_GrandfatherWindow(t *testing.T) {
	h := newHarness(t)
	list := h.createList("L")
	item := h.pricedItem(*list.Id, 10000, "EUR")

	// A live pledge exists while co-buy is still allowed.
	_, c := h.pledge(*list.ShareSlug, *item.Id, 5000, "EUR", "a@example.com")

	// Owner turns co-buy off. The existing pledge is grandfathered (not torn down).
	h.setItemAllowCobuy(t, *item.Id, false)

	// New pledges are now refused (403)...
	resp, _ := h.pledge(*list.ShareSlug, *item.Id, 5000, "EUR", "b@example.com")
	require.Equal(t, http.StatusForbidden, resp.StatusCode)

	// ...but the grandfathered pledge still blocks reserve via #93 cross-track.
	resp, _ = h.reserve(*list.ShareSlug, *item.Id, 1)
	require.Equal(t, http.StatusConflict, resp.StatusCode,
		"a grandfathered live pledge keeps reserve blocked until it drains")

	// Withdraw the pledge → the item finally frees for reserve.
	resp, _ = h.reqH(http.MethodDelete, "/public/contributions/"+*c.ContributionId, h.ownerHost(),
		map[string]string{"X-Capability-Token": *c.CapabilityToken}, nil)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	resp, _ = h.reserve(*list.ShareSlug, *item.Id, 1)
	assert.Equal(t, http.StatusCreated, resp.StatusCode, "with the pledge drained, reserve succeeds")
}
