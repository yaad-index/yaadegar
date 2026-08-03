package api_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/api/gen"
	"github.com/yaad-index/yaadegar/internal/storage"
)

// seedGiver creates a registered giver account in the seeded tenant and returns it
// with a session token, for exercising the authenticated reserve path (cut 3).
func (h *harness) seedGiverSession(email, name string) (storage.User, string) {
	h.t.Helper()
	u, err := h.store.ForTenant(h.tenant).Users().Create(context.Background(), storage.User{
		Name: name, Email: email, Username: &email, Role: storage.RoleGiver,
	})
	require.NoError(h.t, err)
	return u, h.tokenFor(u.ID, h.tenant.ID)
}

// createRegisteredList makes an owner list whose effective reserver tier is
// `registered`, so only the authenticated reserve path can claim its items.
func (h *harness) createRegisteredList(title string) gen.List {
	h.t.Helper()
	resp, body := h.req(http.MethodPost, "/api/v1/lists", h.ownerHost(), h.ownerToken(),
		gen.ListCreate{Title: title, ReserverTier: sptr("registered")})
	require.Equal(h.t, http.StatusCreated, resp.StatusCode, "body: %s", body)
	return decode[gen.List](h.t, body)
}

// reserveAsMe reserves an item through the authenticated account path.
func (h *harness) reserveAsMe(token, slug, itemID string) (*http.Response, []byte) {
	h.t.Helper()
	return h.req(http.MethodPost, "/api/v1/me/reservations", h.ownerHost(), token,
		gen.MyReservationCreate{ShareSlug: slug, ItemId: itemID})
}

// TestRegisteredReserve_AuthedCreatesBoundActiveNoEmail: an authenticated account
// reserving on a registered-tier list gets an active reservation bound to the account,
// with no per-reservation confirmation email and no anonymous capability token, and it
// shows up on the account's own dashboard.
func TestRegisteredReserve_AuthedCreatesBoundActiveNoEmail(t *testing.T) {
	h := newHarness(t)
	giver, tok := h.seedGiverSession("gina@example.test", "Gina")
	list := h.createRegisteredList("Registered")
	item := h.createItem(*list.Id, "Boardgame", 1)

	resp, body := h.reserveAsMe(tok, *list.ShareSlug, *item.Id)
	require.Equal(t, http.StatusCreated, resp.StatusCode, "body: %s", body)
	created := decode[gen.ReservationCreated](t, body)
	assert.Equal(t, gen.ReservationCreatedStatusActive, created.Status)
	assert.Nil(t, created.CapabilityToken, "an account-bound reservation issues no capability token")

	// No confirmation email fires — the account was already verified at registration.
	assert.Never(t, func() bool { return len(h.email.Sent()) > 0 }, 200*time.Millisecond, 20*time.Millisecond)

	// The reservation is bound to the account server-side.
	stored, err := h.store.ForTenant(h.tenant).Reservations().Get(context.Background(), created.ReservationId)
	require.NoError(t, err)
	require.NotNil(t, stored.ReserverUserID)
	assert.Equal(t, giver.ID, *stored.ReserverUserID)

	// And it appears on the account's own dashboard with list/item context.
	resp, body = h.req(http.MethodGet, "/api/v1/me/reservations", h.ownerHost(), tok, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	page := decode[gen.MyReservationPage](t, body)
	require.Equal(t, 1, page.Total)
	require.Len(t, page.Items, 1)
	assert.Equal(t, created.ReservationId, page.Items[0].ReservationId)
	assert.Equal(t, "Boardgame", page.Items[0].ItemName)
	assert.Equal(t, "Registered", page.Items[0].ListTitle)
	assert.Equal(t, *list.ShareSlug, page.Items[0].ShareSlug)
	assert.Equal(t, gen.MyReservationState("active"), page.Items[0].State)
}

// TestRegisteredReserve_AnonymousRejected: the anonymous public reserve path is
// refused (401) on a registered-tier list — it has no account to bind.
func TestRegisteredReserve_AnonymousRejected(t *testing.T) {
	h := newHarness(t)
	list := h.createRegisteredList("Registered")
	item := h.createItem(*list.Id, "Boardgame", 1)

	resp, _ := h.req(http.MethodPost, "/public/"+*list.ShareSlug+"/items/"+*item.Id+"/reservations",
		h.ownerHost(), "", map[string]any{"quantity": 1})
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	// Nothing was reserved.
	assert.Equal(t, gen.Available, h.itemAvailability(t, *list.Id, *item.Id))
}

// TestRegisteredReserve_AuthedWorksOnLowerTier: a registered account may reserve on a
// lower-tier (full_guest) list through the authenticated path too — the tier is a
// floor — and that reservation is bound to the account's dashboard.
func TestRegisteredReserve_AuthedWorksOnLowerTier(t *testing.T) {
	h := newHarness(t)
	_, tok := h.seedGiverSession("gina@example.test", "Gina")
	list := h.createList("Open") // default full_guest tier
	item := h.createItem(*list.Id, "Mug", 1)

	resp, body := h.reserveAsMe(tok, *list.ShareSlug, *item.Id)
	require.Equal(t, http.StatusCreated, resp.StatusCode, "body: %s", body)
	assert.Equal(t, gen.Reserved, h.itemAvailability(t, *list.Id, *item.Id))

	resp, body = h.req(http.MethodGet, "/api/v1/me/reservations", h.ownerHost(), tok, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, 1, decode[gen.MyReservationPage](t, body).Total)
}

// TestMyReservations_Isolation: the dashboard shows only the caller's own
// reservations, never another account's.
func TestMyReservations_Isolation(t *testing.T) {
	h := newHarness(t)
	_, tokA := h.seedGiverSession("a@example.test", "A")
	_, tokB := h.seedGiverSession("b@example.test", "B")
	list := h.createRegisteredList("Registered")
	itemA := h.createItem(*list.Id, "For A", 1)
	itemB := h.createItem(*list.Id, "For B", 1)

	respA, _ := h.reserveAsMe(tokA, *list.ShareSlug, *itemA.Id)
	require.Equal(t, http.StatusCreated, respA.StatusCode)
	respB, _ := h.reserveAsMe(tokB, *list.ShareSlug, *itemB.Id)
	require.Equal(t, http.StatusCreated, respB.StatusCode)

	_, body := h.req(http.MethodGet, "/api/v1/me/reservations", h.ownerHost(), tokA, nil)
	page := decode[gen.MyReservationPage](t, body)
	require.Equal(t, 1, page.Total)
	assert.Equal(t, "For A", page.Items[0].ItemName)
}

// TestDeleteMyReservation_OwnAndForeign: an account releases its own reservation
// (204), but a reservation it does not own — or an anonymous one — is 404.
func TestDeleteMyReservation_OwnAndForeign(t *testing.T) {
	h := newHarness(t)
	_, tokA := h.seedGiverSession("a@example.test", "A")
	_, tokB := h.seedGiverSession("b@example.test", "B")
	list := h.createRegisteredList("Registered")
	item := h.createItem(*list.Id, "Boardgame", 2)

	_, body := h.reserveAsMe(tokA, *list.ShareSlug, *item.Id)
	resID := decode[gen.ReservationCreated](t, body).ReservationId

	// B cannot release A's reservation → 404 (discloses nothing).
	resp, _ := h.req(http.MethodDelete, "/api/v1/me/reservations/"+resID, h.ownerHost(), tokB, nil)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	// A releases its own → 204, item free again.
	resp, _ = h.req(http.MethodDelete, "/api/v1/me/reservations/"+resID, h.ownerHost(), tokA, nil)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Equal(t, gen.Available, h.itemAvailability(t, *list.Id, *item.Id))
}

// TestRegisteredReserve_OwnerViewAnonymity: reusing giver_email/giver_name to carry
// the account identity must not leak it. An owner's views of a registered reservation
// — list items, and JSON + CSV export — must expose neither the reserver's user id nor
// the account email/name (ADR-0002 §5). A regression fence for the anonymity invariant.
func TestRegisteredReserve_OwnerViewAnonymity(t *testing.T) {
	h := newHarness(t)
	giver, tok := h.seedGiverSession("secret-giver@example.test", "Secret Giver")
	list := h.createRegisteredList("Registered")
	item := h.createItem(*list.Id, "Boardgame", 1)

	resp, _ := h.reserveAsMe(tok, *list.ShareSlug, *item.Id)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	secrets := []string{giver.ID, "secret-giver@example.test", "Secret Giver"}
	assertNoSecret := func(what string, body []byte) {
		for _, s := range secrets {
			assert.NotContains(t, string(body), s, "%s leaked reserver identity (%q)", what, s)
		}
	}

	// Owner's item view: only availability + aggregate reserved_quantity.
	_, body := h.req(http.MethodGet, "/api/v1/lists/"+*list.Id+"/items", h.ownerHost(), h.ownerToken(), nil)
	assertNoSecret("list items", body)
	// The item does read as reserved (the aggregate is fine to expose).
	assert.Equal(t, gen.Reserved, h.itemAvailability(t, *list.Id, *item.Id))

	// Owner's JSON export.
	_, body = h.req(http.MethodGet, "/api/v1/lists/"+*list.Id+"/export?format=json", h.ownerHost(), h.ownerToken(), nil)
	assertNoSecret("json export", body)

	// Owner's CSV export.
	_, body = h.req(http.MethodGet, "/api/v1/lists/"+*list.Id+"/export?format=csv", h.ownerHost(), h.ownerToken(), nil)
	assertNoSecret("csv export", body)
}

// TestAuthMethods_RegistrationEnabledReflectsPolicy: the login-methods endpoint
// reports whether self-registration is enabled, so the frontend can gate the register
// affordance and the registered-tier warning (ADR-0012 Decision 5).
func TestAuthMethods_RegistrationEnabledReflectsPolicy(t *testing.T) {
	// Default harness: registration disabled.
	h := newHarness(t)
	_, body := h.req(http.MethodGet, "/api/v1/auth/methods", h.ownerHost(), "", nil)
	assert.False(t, decode[gen.LoginMethods](t, body).RegistrationEnabled)

	// givers_only: enabled.
	h2 := newHarnessRegistration(t, storage.RegistrationGiversOnly)
	_, body = h2.req(http.MethodGet, "/api/v1/auth/methods", h2.ownerHost(), "", nil)
	assert.True(t, decode[gen.LoginMethods](t, body).RegistrationEnabled)
}
