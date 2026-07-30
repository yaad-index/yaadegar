package api_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/api/gen"
	"github.com/yaad-index/yaadegar/internal/storage"
)

// createEmailConfirmedList makes a list whose per-list reserver_tier override is
// email_confirmed (ADR-0007), so reserves on it take the confirm path.
func (h *harness) createEmailConfirmedList(title string) gen.List {
	h.t.Helper()
	l := h.createList(title)
	resp, body := h.req(http.MethodPatch, "/api/v1/lists/"+*l.Id, h.ownerHost(), h.ownerToken(),
		gen.ListUpdate{ReserverTier: sptr("email_confirmed")})
	require.Equal(h.t, http.StatusOK, resp.StatusCode, "body: %s", body)
	return decode[gen.List](h.t, body)
}

// reserveWithEmail reserves one unit as an anonymous giver, supplying giver_email.
func (h *harness) reserveWithEmail(slug, itemID, giverEmail string) (*http.Response, []byte) {
	h.t.Helper()
	body := map[string]any{"quantity": 1}
	if giverEmail != "" {
		body["giver_email"] = giverEmail
	}
	return h.req(http.MethodPost, "/public/"+slug+"/items/"+itemID+"/reservations", h.ownerHost(), "", body)
}

// lastConfirmToken pulls the raw confirmation token out of the most recent email
// the fake sender captured (the giver's only way to get it — it never appears in
// any API response).
func (h *harness) lastConfirmToken() string {
	h.t.Helper()
	sent := h.email.Sent()
	require.NotEmpty(h.t, sent, "expected a confirmation email")
	body := sent[len(sent)-1].Body
	_, tok, ok := strings.Cut(body, "token=")
	require.True(h.t, ok, "no token= in email body: %q", body)
	return tok
}

func (h *harness) confirmReservation(token string) (*http.Response, gen.ReservationConfirmed) {
	h.t.Helper()
	resp, body := h.req(http.MethodPost, "/public/reservations/confirm", h.ownerHost(), "",
		map[string]any{"token": token})
	if resp.StatusCode != http.StatusOK {
		return resp, gen.ReservationConfirmed{}
	}
	return resp, decode[gen.ReservationConfirmed](h.t, body)
}

// TestEmailConfirmedReserveIsPendingNoToken: a reserve on an email_confirmed list
// holds the item as pending (202, status pending_confirmation), issues NO
// capability token, and emails a confirmation link.
func TestEmailConfirmedReserveIsPendingNoToken(t *testing.T) {
	h := newHarness(t)
	list := h.createEmailConfirmedList("Wedding")
	item := h.createItem(*list.Id, "Toaster", 1)

	resp, body := h.reserveWithEmail(*list.ShareSlug, *item.Id, "giver@example.com")
	require.Equal(t, http.StatusAccepted, resp.StatusCode, "body: %s", body)
	created := decode[gen.ReservationCreated](t, body)
	assert.Equal(t, gen.ReservationCreatedStatusPendingConfirmation, created.Status)
	assert.Nil(t, created.CapabilityToken, "no capability token until confirmed")

	// The item is held now (pending counts as reserved on the owner surface).
	assert.Equal(t, gen.Reserved, h.itemAvailability(t, *list.Id, *item.Id))
	// A confirmation email went out.
	require.Len(t, h.email.Sent(), 1)
	assert.Equal(t, "giver@example.com", h.email.Sent()[0].To)
}

// TestEmailConfirmedReserveRequiresValidEmail: the confirm tier needs a
// well-formed giver_email; missing or malformed is a 400 and reserves nothing.
func TestEmailConfirmedReserveRequiresValidEmail(t *testing.T) {
	h := newHarness(t)
	list := h.createEmailConfirmedList("Wedding")
	item := h.createItem(*list.Id, "Toaster", 1)

	resp, _ := h.reserveWithEmail(*list.ShareSlug, *item.Id, "")
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "missing email → 400")

	resp, _ = h.reserveWithEmail(*list.ShareSlug, *item.Id, "not-an-email")
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "malformed email → 400")

	// Nothing was held and no email was sent.
	assert.Equal(t, gen.Available, h.itemAvailability(t, *list.Id, *item.Id))
	assert.Empty(t, h.email.Sent())
}

// TestConfirmActivatesAndIssuesWorkingToken: confirming a pending reservation
// activates it, stamps email_confirmed_at, and returns a capability token that
// actually releases the reservation.
func TestConfirmActivatesAndIssuesWorkingToken(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	list := h.createEmailConfirmedList("Wedding")
	item := h.createItem(*list.Id, "Toaster", 1)

	_, body := h.reserveWithEmail(*list.ShareSlug, *item.Id, "giver@example.com")
	resID := decode[gen.ReservationCreated](t, body).ReservationId

	resp, confirmed := h.confirmReservation(h.lastConfirmToken())
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, resID, confirmed.ReservationId)
	assert.Equal(t, gen.ReservationConfirmedStatusActive, confirmed.Status)
	require.NotNil(t, confirmed.CapabilityToken, "first confirm issues the token")

	// email_confirmed_at is stamped and the state is active (system-only signal).
	stored, err := h.store.ForTenant(h.tenant).Reservations().Get(ctx, resID)
	require.NoError(t, err)
	assert.Equal(t, storage.StateActive, stored.State)
	require.NotNil(t, stored.EmailConfirmedAt)

	// The issued token releases the reservation.
	resp, _ = h.reqH(http.MethodDelete, "/public/reservations/"+resID, h.ownerHost(),
		map[string]string{"X-Capability-Token": *confirmed.CapabilityToken}, nil)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Equal(t, gen.Available, h.itemAvailability(t, *list.Id, *item.Id))
}

// TestConfirmIsIdempotent: re-confirming an already-active reservation returns 200
// status active WITHOUT a new capability token (it cannot be re-issued).
func TestConfirmIsIdempotent(t *testing.T) {
	h := newHarness(t)
	list := h.createEmailConfirmedList("Wedding")
	item := h.createItem(*list.Id, "Toaster", 1)
	_, _ = h.reserveWithEmail(*list.ShareSlug, *item.Id, "giver@example.com")
	tok := h.lastConfirmToken()

	resp, first := h.confirmReservation(tok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NotNil(t, first.CapabilityToken)

	// Second confirm with the same token: still 200, active, but no token.
	resp, second := h.confirmReservation(tok)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, gen.ReservationConfirmedStatusActive, second.Status, "status present and active")
	assert.Nil(t, second.CapabilityToken, "no token re-issued on re-confirm")
}

func TestConfirmUnknownToken404(t *testing.T) {
	h := newHarness(t)
	resp, _ := h.confirmReservation("no-such-token")
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	// Empty token is also a 404 (never a 500).
	resp, _ = h.req(http.MethodPost, "/public/reservations/confirm", h.ownerHost(), "", map[string]any{"token": ""})
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// TestConfirmAfterWindowExpiry410: once the confirm window has elapsed and the
// sweeper expired the pending reservation, confirming returns 410.
func TestConfirmAfterWindowExpiry410(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	list := h.createEmailConfirmedList("Wedding")
	item := h.createItem(*list.Id, "Toaster", 1)
	_, body := h.reserveWithEmail(*list.ShareSlug, *item.Id, "giver@example.com")
	resID := decode[gen.ReservationCreated](t, body).ReservationId
	tok := h.lastConfirmToken()

	// Simulate the confirm-window sweep expiring the pending reservation.
	moved, err := h.store.ForTenant(h.tenant).Reservations().ExpirePending(ctx, resID, h.clk.Now())
	require.NoError(t, err)
	require.True(t, moved)

	resp, _ := h.confirmReservation(tok)
	assert.Equal(t, http.StatusGone, resp.StatusCode)
	// The item freed up.
	assert.Equal(t, gen.Available, h.itemAvailability(t, *list.Id, *item.Id))
}

// TestPendingReservationCannotBeReleasedViaCapabilityPath is the sentinel security
// pin: while pending, the reservation holds a `pending:<id>` token_hash that no
// capability token can hash to — so the release path can never match it, and the
// giver holds no token anyway.
func TestPendingReservationCannotBeReleasedViaCapabilityPath(t *testing.T) {
	h := newHarness(t)
	list := h.createEmailConfirmedList("Wedding")
	item := h.createItem(*list.Id, "Toaster", 1)
	_, body := h.reserveWithEmail(*list.ShareSlug, *item.Id, "giver@example.com")
	resID := decode[gen.ReservationCreated](t, body).ReservationId
	confirmTok := h.lastConfirmToken()

	// Try to release the pending reservation using the confirm token as if it were
	// a capability token → 401 (it is not one).
	resp, _ := h.reqH(http.MethodDelete, "/public/reservations/"+resID, h.ownerHost(),
		map[string]string{"X-Capability-Token": confirmTok}, nil)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	// Try the raw sentinel value itself → also 401.
	resp, _ = h.reqH(http.MethodDelete, "/public/reservations/"+resID, h.ownerHost(),
		map[string]string{"X-Capability-Token": "pending:" + resID}, nil)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	// Still held.
	assert.Equal(t, gen.Reserved, h.itemAvailability(t, *list.Id, *item.Id))
}

// TestConfirmDoesNotRecheckCapacity: a pending reservation holds the slot the whole
// time, so it cannot be crowded out between reserve and confirm — the slot was
// always the pending giver's.
func TestConfirmDoesNotRecheckCapacity(t *testing.T) {
	h := newHarness(t)
	list := h.createEmailConfirmedList("Wedding")
	item := h.createItem(*list.Id, "Single", 1)

	// A holds the last (only) unit as pending.
	_, _ = h.reserveWithEmail(*list.ShareSlug, *item.Id, "a@example.com")
	tokA := h.lastConfirmToken()

	// B cannot reserve — the pending hold already occupies the slot (409).
	resp, _ := h.reserveWithEmail(*list.ShareSlug, *item.Id, "b@example.com")
	require.Equal(t, http.StatusConflict, resp.StatusCode)

	// A confirms → still held, now active. No capacity recheck rejects it.
	resp, confirmed := h.confirmReservation(tokA)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, gen.ReservationConfirmedStatusActive, confirmed.Status)
	assert.Equal(t, gen.Reserved, h.itemAvailability(t, *list.Id, *item.Id))
}

// TestConfirmThenExpireIsNoOp: if confirm wins the race with the confirm-window
// sweep, the later ExpirePending is a benign no-op (the row is already active).
func TestConfirmThenExpireIsNoOp(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	list := h.createEmailConfirmedList("Wedding")
	item := h.createItem(*list.Id, "Toaster", 1)
	_, body := h.reserveWithEmail(*list.ShareSlug, *item.Id, "giver@example.com")
	resID := decode[gen.ReservationCreated](t, body).ReservationId

	resp, _ := h.confirmReservation(h.lastConfirmToken())
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// A late sweep finds it active, not pending → no-op, item stays held.
	moved, err := h.store.ForTenant(h.tenant).Reservations().ExpirePending(ctx, resID, h.clk.Now())
	require.NoError(t, err)
	assert.False(t, moved, "expiry must not touch an already-confirmed reservation")
	assert.Equal(t, gen.Reserved, h.itemAvailability(t, *list.Id, *item.Id))
}

// TestEmailConfirmedNeverLeaksReserverContact is the hard anonymity invariant: the
// reserver's email/name appear on NO owner or public surface, at either the
// pending or the confirmed stage, and email_confirmed_at never surfaces.
func TestEmailConfirmedNeverLeaksReserverContact(t *testing.T) {
	const giverEmail, giverName = "secret-giver@example.com", "Secret Giver"
	h := newHarness(t)
	list := h.createEmailConfirmedList("Wedding")
	item := h.createItem(*list.Id, "Toaster", 1)

	resp, _ := h.req(http.MethodPost, "/public/"+*list.ShareSlug+"/items/"+*item.Id+"/reservations",
		h.ownerHost(), "", map[string]any{"giver_email": giverEmail, "giver_name": giverName})
	require.Equal(t, http.StatusAccepted, resp.StatusCode)

	assertNoReserverLeak := func(stage string) {
		_, ownerBody := h.req(http.MethodGet, "/api/v1/lists/"+*list.Id+"/items", h.ownerHost(), h.ownerToken(), nil)
		_, pubBody := h.req(http.MethodGet, "/public/"+*list.ShareSlug, h.ownerHost(), "", nil)
		for _, s := range []string{giverEmail, giverName, "secret-giver", "email_confirmed_at"} {
			assert.NotContainsf(t, string(ownerBody), s, "%s: owner surface leaked %q", stage, s)
			assert.NotContainsf(t, string(pubBody), s, "%s: public surface leaked %q", stage, s)
		}
	}
	assertNoReserverLeak("pending")

	resp, _ = h.confirmReservation(h.lastConfirmToken())
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assertNoReserverLeak("confirmed")
}

// TestConfirmOnDeletedItemDoesNotError: confirming a pending reservation whose item
// was deleted meanwhile must resolve cleanly (not a 500).
func TestConfirmOnDeletedItemDoesNotError(t *testing.T) {
	h := newHarness(t)
	list := h.createEmailConfirmedList("Wedding")
	item := h.createItem(*list.Id, "Toaster", 1)
	_, _ = h.reserveWithEmail(*list.ShareSlug, *item.Id, "giver@example.com")
	tok := h.lastConfirmToken()

	resp, _ := h.req(http.MethodDelete, "/api/v1/items/"+*item.Id, h.ownerHost(), h.ownerToken(), nil)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	resp, _ = h.confirmReservation(tok)
	assert.Contains(t, []int{http.StatusOK, http.StatusNotFound, http.StatusGone}, resp.StatusCode,
		"confirm on a deleted item must resolve cleanly, not 500")
}

// TestEmailConfirmedSendFailureRollsBack (#86): when the confirmation email can't
// be sent, the provisional hold is rolled back (slot freed immediately) and the
// caller gets a 503 — not a stranded 202. Contrast decay mail, which stays and
// retries; a pending hold is useless without its confirm link.
func TestEmailConfirmedSendFailureRollsBack(t *testing.T) {
	h := newHarness(t)
	list := h.createEmailConfirmedList("Wedding")
	item := h.createItem(*list.Id, "Single", 1) // single slot

	// The confirmation email fails to send.
	h.email.FailWith(errors.New("smtp down"))
	resp, body := h.reserveWithEmail(*list.ShareSlug, *item.Id, "giver@example.com")
	require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode, "body: %s", body)
	assert.NotContains(t, string(body), "giver@example.com", "no reserver email in the error body")

	// The hold was rolled back → the item reads available again on the owner surface.
	assert.Equal(t, gen.Available, h.itemAvailability(t, *list.Id, *item.Id))

	// The slot is genuinely free (no orphaned hold): with sending restored, a fresh
	// reserve on the same single-slot item succeeds (202 pending), which it could
	// not if a phantom hold still occupied the slot.
	h.email.FailWith(nil)
	resp, _ = h.reserveWithEmail(*list.ShareSlug, *item.Id, "giver2@example.com")
	require.Equal(t, http.StatusAccepted, resp.StatusCode)
	assert.Equal(t, gen.Reserved, h.itemAvailability(t, *list.Id, *item.Id))
}

// TestFullGuestReserveUnchanged: a default (full_guest) list still reserves
// immediately — 201, status active, capability token present.
func TestFullGuestReserveUnchanged(t *testing.T) {
	h := newHarness(t)
	list := h.createList("Open List")
	item := h.createItem(*list.Id, "Toaster", 1)

	resp, created := h.reserve(*list.ShareSlug, *item.Id, 1)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	assert.Equal(t, gen.ReservationCreatedStatusActive, created.Status)
	require.NotNil(t, created.CapabilityToken)
	assert.Empty(t, h.email.Sent(), "full_guest sends no confirmation email")
}
