package api_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/api/gen"
)

// pricedItem creates an item with a price for co-buying tests.
func (h *harness) pricedItem(listID string, amountMinor int, currency string) gen.Item {
	h.t.Helper()
	resp, body := h.req(http.MethodPost, "/api/v1/lists/"+listID+"/items", h.ownerHost(), h.ownerToken(),
		gen.ItemCreate{Name: "Espresso machine", Price: &gen.Money{AmountMinor: amountMinor, Currency: currency}})
	require.Equal(h.t, http.StatusCreated, resp.StatusCode, "body: %s", body)
	return decode[gen.Item](h.t, body)
}

// pledge contributes toward an item as an anonymous giver.
func (h *harness) pledge(slug, itemID string, amountMinor int, currency, contact string) (*http.Response, gen.ContributionCreated) {
	h.t.Helper()
	resp, body := h.req(http.MethodPost, "/public/"+slug+"/items/"+itemID+"/contributions", h.ownerHost(), "",
		map[string]any{
			"pledged":       map[string]any{"amount_minor": amountMinor, "currency": currency},
			"contact_email": contact,
		})
	if resp.StatusCode != http.StatusCreated {
		return resp, gen.ContributionCreated{}
	}
	return resp, decode[gen.ContributionCreated](h.t, body)
}

// confirm posts a confirm/decline for a match with the given capability token.
func (h *harness) confirm(matchID, token, decision string) (*http.Response, []byte) {
	h.t.Helper()
	return h.reqH(http.MethodPost, "/public/matches/"+matchID+"/confirm", h.ownerHost(),
		map[string]string{"X-Capability-Token": token}, map[string]any{"decision": decision})
}

// TestCoBuyingHandshake walks the full two-sided flow and asserts the anonymity
// seam at every step: neither contact leaks through any response until both
// parties confirm.
func TestCoBuyingHandshake(t *testing.T) {
	const aEmail, bEmail = "alice-giver@example.com", "bob-giver@example.com"
	h := newHarness(t)
	list := h.createList("Wedding")
	item := h.pricedItem(*list.Id, 40000, "EUR")

	// First pledge: half the price. Only one contributor, so no match yet.
	resp, a := h.pledge(*list.ShareSlug, *item.Id, 20000, "EUR", aEmail)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	assert.Nil(t, a.Match, "a lone pledge proposes no match")
	assert.Empty(t, h.email.Sent(), "no email before a match forms")

	// Second pledge completes coverage with two parties → match proposed.
	resp, bBody := h.pledge(*list.ShareSlug, *item.Id, 20000, "EUR", bEmail)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	require.NotNil(t, bBody.Match, "coverage with two parties proposes a match")
	match := *bBody.Match
	assert.Equal(t, gen.MatchState("proposed"), *match.State)
	assert.Nil(t, match.Contacts, "no contacts revealed at proposal")
	require.Len(t, h.email.Sent(), 2, "both parties emailed on proposal")
	for _, m := range h.email.Sent() {
		assert.NotContains(t, m.Body, aEmail, "proposal email must not reveal a contact")
		assert.NotContains(t, m.Body, bEmail)
	}

	// Either party can poll their own contribution and find the match id — but
	// never the other's contact.
	_, aGet := h.reqH(http.MethodGet, "/public/contributions/"+*a.ContributionId, h.ownerHost(),
		map[string]string{"X-Capability-Token": *a.CapabilityToken}, nil)
	assert.NotContains(t, string(aGet), bEmail)
	contribA := decode[gen.Contribution](t, aGet)
	require.NotNil(t, contribA.MatchId)
	assert.Equal(t, *match.Id, *contribA.MatchId)

	// A confirms — still waiting on B, no contacts yet.
	resp, body := h.confirm(*match.Id, *a.CapabilityToken, "confirm")
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	assert.Nil(t, decode[gen.Match](t, body).Contacts)
	assert.NotContains(t, string(body), bEmail)

	// B confirms — both confirmed, contacts revealed in the response and by email.
	resp, body = h.confirm(*match.Id, *bBody.CapabilityToken, "confirm")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	final := decode[gen.Match](t, body)
	assert.Equal(t, gen.MatchState("both_confirmed"), *final.State)
	require.NotNil(t, final.Contacts)
	emails := make([]string, 0)
	for _, e := range *final.Contacts {
		emails = append(emails, string(e))
	}
	assert.ElementsMatch(t, []string{aEmail, bEmail}, emails)

	// Reveal emails carry the counterpart contact.
	reveal := h.email.Sent()[2:]
	require.Len(t, reveal, 2)
	joined := reveal[0].Body + reveal[1].Body
	assert.Contains(t, joined, aEmail)
	assert.Contains(t, joined, bEmail)
}

func TestCoBuyingLoneFullPriceStaysPending(t *testing.T) {
	h := newHarness(t)
	list := h.createList("List")
	item := h.pricedItem(*list.Id, 30000, "EUR")

	// One pledge covers the whole price — but MinMatchContributions is 2, so it
	// never self-matches; it stays pending.
	resp, c := h.pledge(*list.ShareSlug, *item.Id, 30000, "EUR", "solo@example.com")
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	assert.Nil(t, c.Match)
	assert.Empty(t, h.email.Sent())

	_, body := h.reqH(http.MethodGet, "/public/contributions/"+*c.ContributionId, h.ownerHost(),
		map[string]string{"X-Capability-Token": *c.CapabilityToken}, nil)
	got := decode[gen.Contribution](t, body)
	assert.Equal(t, gen.ContributionStatus("pending"), *got.Status)
	assert.Nil(t, got.MatchId)
}

func TestCoBuyingOverfund409(t *testing.T) {
	h := newHarness(t)
	list := h.createList("List")
	item := h.pricedItem(*list.Id, 10000, "EUR")

	resp, _ := h.pledge(*list.ShareSlug, *item.Id, 8000, "EUR", "a@example.com")
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	// 8000 + 5000 > 10000 → overfund rejected.
	resp, _ = h.pledge(*list.ShareSlug, *item.Id, 5000, "EUR", "b@example.com")
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
}

func TestCoBuyingValidation(t *testing.T) {
	h := newHarness(t)
	list := h.createList("List")
	priced := h.pricedItem(*list.Id, 10000, "EUR")
	noPrice := h.createItem(*list.Id, "Unpriced", 1)

	// No price to co-buy toward.
	resp, _ := h.pledge(*list.ShareSlug, *noPrice.Id, 1000, "EUR", "a@example.com")
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	// Currency mismatch.
	resp, _ = h.pledge(*list.ShareSlug, *priced.Id, 1000, "USD", "a@example.com")
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	// Missing contact.
	resp, body := h.req(http.MethodPost, "/public/"+*list.ShareSlug+"/items/"+*priced.Id+"/contributions",
		h.ownerHost(), "", map[string]any{"pledged": map[string]any{"amount_minor": 1000, "currency": "EUR"}})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "body: %s", body)
}

func TestCoBuyingDeclineReleasesOthers(t *testing.T) {
	h := newHarness(t)
	list := h.createList("List")
	item := h.pricedItem(*list.Id, 20000, "EUR")

	_, a := h.pledge(*list.ShareSlug, *item.Id, 10000, "EUR", "a@example.com")
	_, b := h.pledge(*list.ShareSlug, *item.Id, 10000, "EUR", "b@example.com")
	require.NotNil(t, b.Match)

	// A declines → match declined, B returns to pending (withdrawable again).
	resp, _ := h.confirm(*b.Match.Id, *a.CapabilityToken, "decline")
	require.Equal(t, http.StatusOK, resp.StatusCode)

	_, body := h.reqH(http.MethodGet, "/public/contributions/"+*b.ContributionId, h.ownerHost(),
		map[string]string{"X-Capability-Token": *b.CapabilityToken}, nil)
	got := decode[gen.Contribution](t, body)
	assert.Equal(t, gen.ContributionStatus("pending"), *got.Status)
	assert.Nil(t, got.MatchId, "B is released from the dissolved match")

	// The decliner (still linked to the now-declined match) re-confirming is a 409.
	resp, _ = h.confirm(*b.Match.Id, *a.CapabilityToken, "confirm")
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
}

func TestCoBuyingWithdraw(t *testing.T) {
	h := newHarness(t)
	list := h.createList("List")

	// Withdraw a lone pending pledge → 204.
	item := h.pricedItem(*list.Id, 20000, "EUR")
	_, solo := h.pledge(*list.ShareSlug, *item.Id, 10000, "EUR", "solo@example.com")
	resp, _ := h.reqH(http.MethodDelete, "/public/contributions/"+*solo.ContributionId, h.ownerHost(),
		map[string]string{"X-Capability-Token": *solo.CapabilityToken}, nil)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)

	// Withdraw after both confirmed → 409.
	item2 := h.pricedItem(*list.Id, 20000, "EUR")
	_, a := h.pledge(*list.ShareSlug, *item2.Id, 10000, "EUR", "a@example.com")
	_, b := h.pledge(*list.ShareSlug, *item2.Id, 10000, "EUR", "b@example.com")
	require.NotNil(t, b.Match)
	_, _ = h.confirm(*b.Match.Id, *a.CapabilityToken, "confirm")
	_, _ = h.confirm(*b.Match.Id, *b.CapabilityToken, "confirm")

	resp, _ = h.reqH(http.MethodDelete, "/public/contributions/"+*a.ContributionId, h.ownerHost(),
		map[string]string{"X-Capability-Token": *a.CapabilityToken}, nil)
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
}

func TestConfirmMatchAuth(t *testing.T) {
	h := newHarness(t)
	list := h.createList("List")
	item := h.pricedItem(*list.Id, 20000, "EUR")
	_, a := h.pledge(*list.ShareSlug, *item.Id, 10000, "EUR", "a@example.com")
	_, b := h.pledge(*list.ShareSlug, *item.Id, 10000, "EUR", "b@example.com")
	require.NotNil(t, b.Match)

	// Missing token → 401.
	resp, _ := h.reqH(http.MethodPost, "/public/matches/"+*b.Match.Id+"/confirm", h.ownerHost(),
		nil, map[string]any{"decision": "confirm"})
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	// Unknown match → 404.
	resp, _ = h.confirm("no-such-match", *a.CapabilityToken, "confirm")
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	// A token not belonging to the match → 404.
	item2 := h.pricedItem(*list.Id, 5000, "EUR")
	_, outsider := h.pledge(*list.ShareSlug, *item2.Id, 5000, "EUR", "outsider@example.com")
	resp, _ = h.confirm(*b.Match.Id, *outsider.CapabilityToken, "confirm")
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}
