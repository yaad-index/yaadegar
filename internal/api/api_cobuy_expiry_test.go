package api_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/api/gen"
	"github.com/yaad-index/yaadegar/internal/storage"
)

// expireMatch drives a proposed match through the store's auto-expiry transition
// (the same call the #101 sweeper makes), so the handler-level effects can be
// asserted without running the ticker.
func (h *harness) expireMatch(itemID, matchID string) {
	h.t.Helper()
	moved, err := h.store.ForTenant(h.tenant).Matches().
		ExpireIfProposed(context.Background(), itemID, matchID, h.clk.Now())
	require.NoError(h.t, err)
	require.True(h.t, moved, "match should have been proposed and now expired")
}

// After the sweep expires a match, GET on a participating contribution reports the
// terminal `expired` status — the signal the list page keys on to drop the stale
// capability so the pledged panel doesn't show a dead "you're chipping in".
func TestExpiredContributionReportsExpiredStatus(t *testing.T) {
	h := newHarness(t)
	list := h.createList("L")
	item := h.pricedItem(*list.Id, 10000, "EUR")
	_, a := h.pledge(*list.ShareSlug, *item.Id, 5000, "EUR", "a@example.com")
	_, b := h.pledge(*list.ShareSlug, *item.Id, 5000, "EUR", "b@example.com")
	require.NotNil(t, b.Match)

	h.expireMatch(*item.Id, *b.Match.Id)

	_, body := h.reqH(http.MethodGet, "/public/contributions/"+*a.ContributionId, h.ownerHost(),
		map[string]string{"X-Capability-Token": *a.CapabilityToken}, nil)
	got := decode[gen.Contribution](t, body)
	require.NotNil(t, got.Status)
	assert.Equal(t, gen.ContributionStatus("expired"), *got.Status)
}

// Withdrawing a contribution whose match already auto-expired must NOT resurrect the
// sibling pledges: the withdraw path only dissolves a still-proposed match, so the
// terminal siblings stay expired (and the item stays free for reserve under #93).
func TestWithdrawAfterExpiryDoesNotResurrectSiblings(t *testing.T) {
	h := newHarness(t)
	list := h.createList("L")
	item := h.pricedItem(*list.Id, 10000, "EUR")
	_, a := h.pledge(*list.ShareSlug, *item.Id, 5000, "EUR", "a@example.com")
	_, b := h.pledge(*list.ShareSlug, *item.Id, 5000, "EUR", "b@example.com")
	require.NotNil(t, b.Match)

	h.expireMatch(*item.Id, *b.Match.Id)

	// Withdraw a's (already-expired) contribution — allowed, a plain delete.
	resp, _ := h.reqH(http.MethodDelete, "/public/contributions/"+*a.ContributionId, h.ownerHost(),
		map[string]string{"X-Capability-Token": *a.CapabilityToken}, nil)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	// b must NOT have been flipped back to pending by a dissolve — it stays expired.
	bStored, err := h.store.ForTenant(h.tenant).Contributions().Get(context.Background(), *b.ContributionId)
	require.NoError(t, err)
	assert.Equal(t, storage.ContributionExpired, bStored.Status, "sibling pledge must stay terminal")

	// And the item is still free for reserve (no live co-buy resurrected).
	resp, _ = h.reserve(*list.ShareSlug, *item.Id, 1)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)
}
