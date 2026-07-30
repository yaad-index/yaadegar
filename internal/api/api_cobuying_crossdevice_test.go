package api_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/api/gen"
)

// getMatch reads a match via the read endpoint with a token (cap or scoped).
func (h *harness) getMatch(matchID, token string) (*http.Response, gen.Match) {
	h.t.Helper()
	resp, body := h.reqH(http.MethodGet, "/public/matches/"+matchID, h.ownerHost(),
		map[string]string{"X-Capability-Token": token}, nil)
	if resp.StatusCode != http.StatusOK {
		return resp, gen.Match{}
	}
	return resp, decode[gen.Match](h.t, body)
}

// scopedTokenFor pulls the scoped match-action token out of the proposal email
// sent to contact (the giver's only way to get it — it never appears in an API
// response). It returns the most recent one.
func (h *harness) scopedTokenFor(contact string) string {
	h.t.Helper()
	var tok string
	for _, m := range h.email.Sent() {
		if m.To == contact && strings.Contains(m.Body, "/cobuy/") {
			if _, t, ok := strings.Cut(m.Body, "?t="); ok {
				tok = strings.TrimSpace(t)
			}
		}
	}
	require.NotEmpty(h.t, tok, "no scoped-token email for %s", contact)
	return tok
}

// twoPartyMatch forms a proposed match over two pledges covering a single-price
// item, returning the two ContributionCreated + the match id.
func (h *harness) twoPartyMatch(t *testing.T) (gen.ContributionCreated, gen.ContributionCreated, string) {
	t.Helper()
	list := h.createList("Gifts")
	item := h.pricedItem(*list.Id, 10000, "EUR")
	_, a := h.pledge(*list.ShareSlug, *item.Id, 5000, "EUR", "a@example.com")
	_, b := h.pledge(*list.ShareSlug, *item.Id, 5000, "EUR", "b@example.com")
	require.NotNil(t, b.Match, "coverage proposes a match")
	return a, b, *b.Match.Id
}

// TestCrossDeviceMatchEmailCarriesScopedToken: a proposed match emails each party a
// /cobuy/<matchId>?t=<scoped-token> link, and each contribution gets a stored
// match-action token hash.
func TestCrossDeviceMatchEmailCarriesScopedToken(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	a, b, matchID := h.twoPartyMatch(t)

	for _, contact := range []string{"a@example.com", "b@example.com"} {
		var found bool
		for _, m := range h.email.Sent() {
			if m.To == contact && strings.Contains(m.Body, "/cobuy/"+matchID+"?t=") {
				found = true
			}
		}
		assert.Truef(t, found, "%s got a /cobuy link for the match", contact)
	}

	cs := h.store.ForTenant(h.tenant).Contributions()
	for _, id := range []string{*a.ContributionId, *b.ContributionId} {
		c, err := cs.Get(ctx, id)
		require.NoError(t, err)
		assert.NotEmpty(t, c.MatchActionTokenHash, "each participant gets a scoped token")
	}
}

// TestGetMatchViaCapAndScopedToken: the read endpoint works with either token, and
// withholds contacts until both_confirmed.
func TestGetMatchViaCapAndScopedToken(t *testing.T) {
	h := newHarness(t)
	a, b, matchID := h.twoPartyMatch(t)

	// Proposed: both auth modes read state; NO contacts.
	resp, m := h.getMatch(matchID, *a.CapabilityToken)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, gen.MatchState("proposed"), *m.State)
	assert.Nil(t, m.Contacts, "no contacts while proposed (cap token)")

	resp, m = h.getMatch(matchID, h.scopedTokenFor("a@example.com"))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Nil(t, m.Contacts, "no contacts while proposed (scoped token)")

	// Both confirm → both_confirmed reveals all contacts on the read.
	h.confirm(matchID, *a.CapabilityToken, "confirm")
	h.confirm(matchID, *b.CapabilityToken, "confirm")
	resp, m = h.getMatch(matchID, *a.CapabilityToken)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, gen.MatchState("both_confirmed"), *m.State)
	require.NotNil(t, m.Contacts)
	assert.ElementsMatch(t, []string{"a@example.com", "b@example.com"}, stringsOf(*m.Contacts))
}

// TestGetMatchForeignCapTokenRejected is the participation fence: a valid capability
// token belonging to a DIFFERENT match cannot read this one (must be 404, not a leak).
func TestGetMatchForeignCapTokenRejected(t *testing.T) {
	h := newHarness(t)
	_, _, matchID := h.twoPartyMatch(t)

	// A separate list/item/pledge whose capability token is valid but unrelated.
	list2 := h.createList("Other")
	item2 := h.pricedItem(*list2.Id, 4000, "EUR")
	_, foreign := h.pledge(*list2.ShareSlug, *item2.Id, 4000, "EUR", "foreign@example.com")

	resp, _ := h.getMatch(matchID, *foreign.CapabilityToken)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode,
		"a foreign but valid cap token cannot read this match")
}

// TestGetMatchExpiredScopedToken410: a scoped token past its expiry is a 410 on read.
func TestGetMatchExpiredScopedToken410(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	a, _, matchID := h.twoPartyMatch(t)
	scoped := h.scopedTokenFor("a@example.com")

	// Force the stored token to be already expired (the harness window is unset, so
	// tokens don't expire on their own).
	cs := h.store.ForTenant(h.tenant).Contributions()
	c, err := cs.Get(ctx, *a.ContributionId)
	require.NoError(t, err)
	past := testClockStart.Add(-time.Hour)
	require.NoError(t, cs.SetMatchActionToken(ctx, *a.ContributionId, c.MatchActionTokenHash, &past))

	resp, _ := h.getMatch(matchID, scoped)
	assert.Equal(t, http.StatusGone, resp.StatusCode)
}

// TestConfirmViaScopedToken: the scoped token confirms a match (dual-auth parity),
// so a cross-device giver can complete the group buy.
func TestConfirmViaScopedToken(t *testing.T) {
	h := newHarness(t)
	a, b, matchID := h.twoPartyMatch(t)

	// A confirms with the scoped token, B with the capability token → both_confirmed.
	resp, _ := h.confirm(matchID, h.scopedTokenFor("a@example.com"), "confirm")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp, body := h.confirm(matchID, *b.CapabilityToken, "confirm")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, gen.MatchState("both_confirmed"), *decode[gen.Match](t, body).State)
	_ = a
}

// TestWithdrawViaScopedTokenRejected: the scoped token is confirm/decline-only — it
// cannot withdraw a pledge (the withdraw path accepts only the capability token).
func TestWithdrawViaScopedTokenRejected(t *testing.T) {
	h := newHarness(t)
	a, _, _ := h.twoPartyMatch(t)

	resp, _ := h.reqH(http.MethodDelete, "/public/contributions/"+*a.ContributionId, h.ownerHost(),
		map[string]string{"X-Capability-Token": h.scopedTokenFor("a@example.com")}, nil)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
		"a scoped match-action token cannot withdraw")
}

// TestScopedTokenClearedOnDissolution: when a decline dissolves the match, the
// released participants' scoped tokens are cleared (no stale residue for a re-pledge).
func TestScopedTokenClearedOnDissolution(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	a, b, matchID := h.twoPartyMatch(t)

	resp, _ := h.confirm(matchID, *a.CapabilityToken, "decline")
	require.Equal(t, http.StatusOK, resp.StatusCode)

	cs := h.store.ForTenant(h.tenant).Contributions()
	for _, id := range []string{*a.ContributionId, *b.ContributionId} {
		c, err := cs.Get(ctx, id)
		require.NoError(t, err)
		assert.Emptyf(t, c.MatchActionTokenHash, "scoped token cleared on dissolution for %s", id)
	}
}

// stringsOf converts a slice of openapi email types to plain strings.
func stringsOf[T ~string](in []T) []string {
	out := make([]string, len(in))
	for i, v := range in {
		out[i] = string(v)
	}
	return out
}
