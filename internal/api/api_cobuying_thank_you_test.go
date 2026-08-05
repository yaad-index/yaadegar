package api_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/email"
)

// #113: the owner→giver thank-you note (#22) also reaches co-buy contributors.
// When a co-buy match reaches both_confirmed (the reveal point), each participating
// contributor gets the same note — item + thanks only, never a giver identity —
// once per contributor, best-effort, and only at both_confirmed.

// thankYous returns the thank-you notes among the sent mail, identified by the
// co-buy subject. Co-buy proposal ("A co-buying match is proposed") and reveal
// ("Your co-buying match is confirmed") mail carry other subjects, so this
// isolates the thank-you fan-out from the surrounding co-buy emails.
func thankYous(sent []email.Message) []email.Message {
	var out []email.Message
	for _, m := range sent {
		if m.Subject == "Thank you for chipping in on Espresso machine" {
			out = append(out, m)
		}
	}
	return out
}

// Both contributors get the thank-you once at both_confirmed, and it stays
// anonymous: the note names the item (via {item}) and never a giver's contact.
func TestCoBuyThankYou_SentToEachContributorAtBothConfirmed(t *testing.T) {
	const aEmail, bEmail = "alice-giver@example.com", "bob-giver@example.com"
	h := newHarness(t)
	list := h.createList("Wedding")
	item := h.pricedItem(*list.Id, 40000, "EUR")
	h.setListThankYou(t, *list.Id, "Thanks for chipping in on {item}! — the host")

	_, a := h.pledge(*list.ShareSlug, *item.Id, 20000, "EUR", aEmail)
	_, b := h.pledge(*list.ShareSlug, *item.Id, 20000, "EUR", bEmail)
	require.NotNil(t, b.Match)

	// One party confirms — not both_confirmed yet, so no thank-you fires.
	resp, body := h.confirm(*b.Match.Id, *a.CapabilityToken, "confirm")
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	assert.Empty(t, thankYous(h.email.Sent()), "no thank-you before the match is both_confirmed")

	// Second confirm completes the match → thank-you fires once per contributor.
	resp, body = h.confirm(*b.Match.Id, *b.CapabilityToken, "confirm")
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)

	notes := thankYous(h.email.Sent())
	require.Len(t, notes, 2, "one thank-you per participating contributor")
	got := map[string]string{}
	for _, m := range notes {
		got[m.To] = m.Body
		assert.Equal(t, "Thank you for chipping in on Espresso machine", m.Subject,
			"co-buy-fitting subject, {item} substituted")
		assert.Equal(t, "Thanks for chipping in on Espresso machine! — the host", m.Body,
			"{item} substituted, owner-authored body")
		// Anonymity: the note must not carry any giver's identity.
		assert.NotContains(t, m.Body, aEmail)
		assert.NotContains(t, m.Body, bEmail)
	}
	assert.Equal(t, map[string]string{
		aEmail: "Thanks for chipping in on Espresso machine! — the host",
		bEmail: "Thanks for chipping in on Espresso machine! — the host",
	}, got, "each contributor's own contact is the recipient, once each")
}

// The per-item / list two-level resolution carries over: with no template
// configured, the reveal still happens but no thank-you is sent.
func TestCoBuyThankYou_NoNoteWhenUnconfigured(t *testing.T) {
	h := newHarness(t)
	list := h.createList("Wedding")
	item := h.pricedItem(*list.Id, 20000, "EUR")

	_, a := h.pledge(*list.ShareSlug, *item.Id, 10000, "EUR", "a@example.com")
	_, b := h.pledge(*list.ShareSlug, *item.Id, 10000, "EUR", "b@example.com")
	require.NotNil(t, b.Match)
	_, _ = h.confirm(*b.Match.Id, *a.CapabilityToken, "confirm")
	resp, _ := h.confirm(*b.Match.Id, *b.CapabilityToken, "confirm")
	require.Equal(t, http.StatusOK, resp.StatusCode)

	assert.Empty(t, thankYous(h.email.Sent()), "no template configured → no thank-you note")
}

// Best-effort: a thank-you send failure must not break the match transition — the
// completing confirm still returns both_confirmed with the revealed contacts.
func TestCoBuyThankYou_BestEffortSendFailure(t *testing.T) {
	h := newHarness(t)
	list := h.createList("Wedding")
	item := h.pricedItem(*list.Id, 20000, "EUR")
	h.setListThankYou(t, *list.Id, "thanks for {item}")

	_, a := h.pledge(*list.ShareSlug, *item.Id, 10000, "EUR", "a@example.com")
	_, b := h.pledge(*list.ShareSlug, *item.Id, 10000, "EUR", "b@example.com")
	require.NotNil(t, b.Match)
	_, _ = h.confirm(*b.Match.Id, *a.CapabilityToken, "confirm")

	// Every send fails from here — the completing confirm must still succeed.
	h.email.FailWith(errors.New("smtp down"))
	resp, body := h.confirm(*b.Match.Id, *b.CapabilityToken, "confirm")
	require.Equal(t, http.StatusOK, resp.StatusCode, "a failed thank-you must not fail the match: %s", body)
}
