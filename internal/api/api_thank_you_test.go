package api_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #22: owner→giver thank-you note. Server emails the owner's template (item + thanks
// only, no giver identity) to a reserver's contact when the reservation goes active.

func (h *harness) setListThankYou(t *testing.T, listID, tmpl string) {
	t.Helper()
	resp, body := h.req(http.MethodPatch, "/api/v1/lists/"+listID, h.ownerHost(), h.ownerToken(),
		map[string]any{"thank_you_template": tmpl})
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
}

// setItemThankYou patches the per-item override; pass a *string (nil sends JSON null).
func (h *harness) setItemThankYou(t *testing.T, itemID string, val any) {
	t.Helper()
	resp, body := h.req(http.MethodPatch, "/api/v1/items/"+itemID, h.ownerHost(), h.ownerToken(),
		map[string]any{"thank_you_template": val})
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
}

// reserveGuest reserves as a full_guest carrying a giver name + email.
func (h *harness) reserveGuest(t *testing.T, slug, itemID, giverName, giverEmail string) {
	t.Helper()
	resp, _ := h.req(http.MethodPost, "/public/"+slug+"/items/"+itemID+"/reservations", h.ownerHost(), "",
		map[string]any{"quantity": 1, "giver_name": giverName, "giver_email": giverEmail})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
}

// A full_guest reserver with an email gets the list's thank-you note; the mail names
// the item (via {item}) and NEVER the giver — the owner stays blind to who reserved.
func TestThankYou_FullGuestSentAndAnonymous(t *testing.T) {
	h := newHarness(t)
	list := h.createList("L")
	item := h.createItem(*list.Id, "Blender", 1)
	h.setListThankYou(t, *list.Id, "Thanks so much for {item}! — the host")

	h.reserveGuest(t, *list.ShareSlug, *item.Id, "Grandma Ada", "ada@example.com")

	sent := h.email.Sent()
	require.Len(t, sent, 1)
	m := sent[0]
	assert.Equal(t, "ada@example.com", m.To)
	assert.Equal(t, "Thank you for reserving Blender", m.Subject)
	assert.Contains(t, m.Body, "Blender", "{item} is substituted with the item name")
	assert.Equal(t, "Thanks so much for Blender! — the host", m.Body)
	// Anonymity: the note must not leak the reserver's identity back to the owner.
	assert.NotContains(t, m.Body, "Grandma Ada")
	assert.NotContains(t, m.Subject, "Grandma Ada")
	assert.NotContains(t, m.Body, "ada@example.com")
}

// An email_confirmed reserver gets the thank-you on CONFIRM (when the hold becomes
// active), not on the initial reserve.
func TestThankYou_EmailConfirmedSentOnConfirm(t *testing.T) {
	h := newHarness(t)
	list := h.createEmailConfirmedList("L")
	item := h.createItem(*list.Id, "Kettle", 1)
	h.setListThankYou(t, *list.Id, "Thank you for {item}.")

	resp, _ := h.reserveWithEmail(*list.ShareSlug, *item.Id, "reserver@example.com")
	require.Equal(t, http.StatusAccepted, resp.StatusCode)
	// So far only the confirm-link email; no thank-you yet.
	require.Len(t, h.email.Sent(), 1)
	require.Equal(t, "Confirm your reservation", h.email.Sent()[0].Subject)

	resp2, _ := h.confirmReservation(h.lastConfirmToken())
	require.Equal(t, http.StatusOK, resp2.StatusCode)

	sent := h.email.Sent()
	require.Len(t, sent, 2)
	assert.Equal(t, "Thank you for Kettle.", sent[1].Body)
	assert.Equal(t, "reserver@example.com", sent[1].To)
}

// The per-item override beats the list default.
func TestThankYou_ItemOverrideBeatsList(t *testing.T) {
	h := newHarness(t)
	list := h.createList("L")
	item := h.createItem(*list.Id, "Lamp", 1)
	h.setListThankYou(t, *list.Id, "list default {item}")
	h.setItemThankYou(t, *item.Id, "special {item} note")

	h.reserveGuest(t, *list.ShareSlug, *item.Id, "G", "g@example.com")
	require.Len(t, h.email.Sent(), 1)
	assert.Equal(t, "special Lamp note", h.email.Sent()[0].Body)
}

// Per-item opt-out: an explicit empty-string item override suppresses the note
// even when the list has a non-empty default (distinct from null = inherit).
func TestThankYou_EmptyItemOverrideSuppressesEvenWithListDefault(t *testing.T) {
	h := newHarness(t)
	list := h.createList("L")
	item := h.createItem(*list.Id, "Vase", 1)
	h.setListThankYou(t, *list.Id, "thanks for {item}")
	h.setItemThankYou(t, *item.Id, "") // explicit opt-out

	h.reserveGuest(t, *list.ShareSlug, *item.Id, "G", "g@example.com")
	assert.Empty(t, h.email.Sent(), "empty-string item override suppresses the note")

	// And a null override clears back to inheriting the list default → note sends.
	item2 := h.createItem(*list.Id, "Clock", 1)
	h.setItemThankYou(t, *item2.Id, "temp")
	h.setItemThankYou(t, *item2.Id, nil) // null → inherit
	h.reserveGuest(t, *list.ShareSlug, *item2.Id, "G", "g2@example.com")
	require.Len(t, h.email.Sent(), 1)
	assert.Equal(t, "thanks for Clock", h.email.Sent()[0].Body)
}

// No note is sent when neither level configures one, or the reserver left no email.
func TestThankYou_NoSendWhenUnconfiguredOrNoEmail(t *testing.T) {
	h := newHarness(t)
	list := h.createList("L")
	unconfigured := h.createItem(*list.Id, "Book", 1)
	noEmail := h.createItem(*list.Id, "Pen", 1)

	// No template anywhere → no send.
	h.reserveGuest(t, *list.ShareSlug, *unconfigured.Id, "G", "g@example.com")
	assert.Empty(t, h.email.Sent())

	// Template set, but a reserver with no email → nothing to send to.
	h.setListThankYou(t, *list.Id, "thanks {item}")
	resp, _ := h.reserve(*list.ShareSlug, *noEmail.Id, 1) // no giver email
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	assert.Empty(t, h.email.Sent())
}

// Best-effort: a thank-you send failure is swallowed — the reservation still succeeds.
func TestThankYou_BestEffortSendFailure(t *testing.T) {
	h := newHarness(t)
	list := h.createList("L")
	item := h.createItem(*list.Id, "Chair", 1)
	h.setListThankYou(t, *list.Id, "thanks for {item}")
	h.email.FailWith(errors.New("smtp down"))

	resp, _ := h.req(http.MethodPost, "/public/"+*list.ShareSlug+"/items/"+*item.Id+"/reservations",
		h.ownerHost(), "", map[string]any{"quantity": 1, "giver_email": "g@example.com"})
	assert.Equal(t, http.StatusCreated, resp.StatusCode, "a failed thank-you must not fail the reserve")
}
