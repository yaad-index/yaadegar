package api_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/api/gen"
)

// #111: PATCH override fields are three-state — absent leaves the value, explicit
// null clears it (to inherit, or to no value), a value sets it. These tests drive
// the clear (null) path, which was previously unreachable, and assert the RESOLVED
// effective behaviour, not just that the column went null.

// getList reads the owner's list back.
func (h *harness) getList(t *testing.T, listID string) gen.List {
	t.Helper()
	resp, body := h.req(http.MethodGet, "/api/v1/lists/"+listID, h.ownerHost(), h.ownerToken(), nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	return decode[gen.List](t, body)
}

// Item allow_cobuy: an explicit null clears the override so the item resolves back
// to the list default — proven end-to-end via the contribute gate (#100) flipping
// from 403 (opted out) to 201 (inherits the enabled list default).
func TestClearSemantics_ItemAllowCobuy_ResolvedRevert(t *testing.T) {
	h := newHarness(t)
	list := h.createList("L")
	item := h.pricedItem(*list.Id, 10000, "EUR")

	// Opt the item out → contribute is refused, public resolves false.
	h.setItemAllowCobuy(t, *item.Id, false)
	resp, _ := h.pledge(*list.ShareSlug, *item.Id, 5000, "EUR", "a@example.com")
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	assert.False(t, *h.publicItem(t, *list.ShareSlug, *item.Id).AllowCobuy)

	// Absent (patch an unrelated field) leaves the override in place → still 403.
	resp, body := h.req(http.MethodPatch, "/api/v1/items/"+*item.Id, h.ownerHost(), h.ownerToken(),
		map[string]any{"name": "Renamed"})
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	resp, _ = h.pledge(*list.ShareSlug, *item.Id, 5000, "EUR", "b@example.com")
	require.Equal(t, http.StatusForbidden, resp.StatusCode, "absent allow_cobuy must not clear the override")

	// Explicit null clears it → the item inherits the list default (enabled), so
	// public resolves true and a pledge now succeeds.
	resp, body = h.req(http.MethodPatch, "/api/v1/items/"+*item.Id, h.ownerHost(), h.ownerToken(),
		map[string]any{"allow_cobuy": nil})
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	assert.Nil(t, decode[gen.Item](t, body).AllowCobuy, "owner view shows the override cleared (inheriting)")
	assert.True(t, *h.publicItem(t, *list.ShareSlug, *item.Id).AllowCobuy, "resolves to the list default")
	resp, _ = h.pledge(*list.ShareSlug, *item.Id, 5000, "EUR", "c@example.com")
	assert.Equal(t, http.StatusCreated, resp.StatusCode, "cleared override → inherits enabled default → 201")
}

// List reserver_tier / decay_days / reserver_confirm_window: an explicit null
// clears the override, so the list reads back as inheriting the instance default
// (null in the response). For decay_days / confirm_window this also exercises the
// nil→inherit-sentinel storage write, previously unreachable via PATCH.
func TestClearSemantics_ListOverrides_ClearToInherit(t *testing.T) {
	h := newHarness(t)

	created := h.createList("L")

	// Set all three overrides.
	resp, body := h.req(http.MethodPatch, "/api/v1/lists/"+*created.Id, h.ownerHost(), h.ownerToken(),
		map[string]any{"reserver_tier": "email_confirmed", "decay_days": 5, "reserver_confirm_window": 30})
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	set := decode[gen.List](t, body)
	require.NotNil(t, set.ReserverTier)
	assert.Equal(t, "email_confirmed", *set.ReserverTier)
	require.NotNil(t, set.DecayDays)
	assert.Equal(t, 5, *set.DecayDays)
	require.NotNil(t, set.ReserverConfirmWindow)
	assert.Equal(t, 30, *set.ReserverConfirmWindow)

	// Clear all three with explicit null → each reads back as inheriting (null).
	resp, body = h.req(http.MethodPatch, "/api/v1/lists/"+*created.Id, h.ownerHost(), h.ownerToken(),
		map[string]any{"reserver_tier": nil, "decay_days": nil, "reserver_confirm_window": nil})
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	cleared := decode[gen.List](t, body)
	assert.Nil(t, cleared.ReserverTier, "reserver_tier cleared to inherit")
	assert.Nil(t, cleared.DecayDays, "decay_days cleared to inherit (nil→sentinel round-trip)")
	assert.Nil(t, cleared.ReserverConfirmWindow, "reserver_confirm_window cleared to inherit")

	// The clear persists (a fresh read agrees).
	reread := h.getList(t, *created.Id)
	assert.Nil(t, reread.ReserverTier)
	assert.Nil(t, reread.DecayDays)
	assert.Nil(t, reread.ReserverConfirmWindow)
}

// event_date is a plain nullable (no parent to inherit): an explicit null clears it
// to NO event date, and it reads back absent — not "reverts to a default".
func TestClearSemantics_ListEventDate_ClearToNoDate(t *testing.T) {
	h := newHarness(t)
	created := h.createList("L")

	resp, body := h.req(http.MethodPatch, "/api/v1/lists/"+*created.Id, h.ownerHost(), h.ownerToken(),
		map[string]any{"event_date": "2027-12-31"})
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	require.NotNil(t, decode[gen.List](t, body).EventDate)

	resp, body = h.req(http.MethodPatch, "/api/v1/lists/"+*created.Id, h.ownerHost(), h.ownerToken(),
		map[string]any{"event_date": nil})
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	assert.Nil(t, decode[gen.List](t, body).EventDate, "event_date null clears to no date (reads back absent)")
	assert.Nil(t, h.getList(t, *created.Id).EventDate)
}

// Absent leaves an existing override untouched (the merge-patch guarantee).
func TestClearSemantics_AbsentLeavesUnchanged(t *testing.T) {
	h := newHarness(t)
	created := h.createList("L")

	_, _ = h.req(http.MethodPatch, "/api/v1/lists/"+*created.Id, h.ownerHost(), h.ownerToken(),
		map[string]any{"decay_days": 7})
	// Patch only the title — decay_days must survive.
	resp, body := h.req(http.MethodPatch, "/api/v1/lists/"+*created.Id, h.ownerHost(), h.ownerToken(),
		map[string]any{"title": "Renamed"})
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	got := decode[gen.List](t, body)
	require.NotNil(t, got.DecayDays)
	assert.Equal(t, 7, *got.DecayDays, "absent decay_days must not clear the existing override")
}
