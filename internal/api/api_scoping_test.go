package api_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/api/gen"
	"github.com/yaad-index/yaadegar/internal/storage"
)

// createList is a small helper: create a list as the seeded owner, return it.
func (h *harness) createList(title string) gen.List {
	h.t.Helper()
	resp, body := h.req(http.MethodPost, "/api/v1/lists", h.ownerHost(), h.ownerToken(), gen.ListCreate{Title: title})
	require.Equal(h.t, http.StatusCreated, resp.StatusCode, "body: %s", body)
	return decode[gen.List](h.t, body)
}

func (h *harness) createItem(listID, name string, qtyWanted int) gen.Item {
	h.t.Helper()
	body := gen.ItemCreate{Name: name}
	if qtyWanted > 0 {
		body.QuantityWanted = &qtyWanted
	}
	resp, b := h.req(http.MethodPost, "/api/v1/lists/"+listID+"/items", h.ownerHost(), h.ownerToken(), body)
	require.Equal(h.t, http.StatusCreated, resp.StatusCode, "body: %s", b)
	return decode[gen.Item](h.t, b)
}

// TestTenantScopingThroughAPI proves isolation holds end-to-end through the HTTP
// surface: another tenant's owner, on their own host, cannot read this tenant's
// list even with its id.
func TestTenantScopingThroughAPI(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	list := h.createList("Alice's list")

	// A second tenant with its own owner.
	tenB, err := h.store.CreateTenant(ctx, storage.Tenant{Subdomain: "bob"})
	require.NoError(t, err)
	ownerB, err := h.store.ForTenant(tenB).Users().Create(ctx, storage.User{Name: "Bob"})
	require.NoError(t, err)
	bobHost, bobTok := "bob."+baseDomain, ownerB.ID

	// Bob cannot see Alice's list by id.
	resp, _ := h.req(http.MethodGet, "/api/v1/lists/"+*list.Id, bobHost, bobTok, nil)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	// Bob's own list collection is empty.
	resp, body := h.req(http.MethodGet, "/api/v1/lists", bobHost, bobTok, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Zero(t, *decode[gen.ListPage](t, body).Total)

	// Alice's own list is still there.
	resp, _ = h.req(http.MethodGet, "/api/v1/lists/"+*list.Id, h.ownerHost(), h.ownerToken(), nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestPublicListView(t *testing.T) {
	h := newHarness(t)
	list := h.createList("Shared")
	h.createItem(*list.Id, "Gift", 1)
	slug := *list.ShareSlug

	// Anonymous (no auth) view by slug.
	resp, body := h.req(http.MethodGet, "/public/"+slug, h.ownerHost(), "", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	pub := decode[gen.PublicList](t, body)
	assert.Equal(t, "Shared", *pub.Title)
	require.Len(t, *pub.Items, 1)
	assert.Equal(t, gen.Available, *(*pub.Items)[0].Availability)

	// Unknown slug → 404.
	resp, _ = h.req(http.MethodGet, "/public/nope", h.ownerHost(), "", nil)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	// Deactivate the list → public view is Gone.
	resp, _ = h.req(http.MethodPatch, "/api/v1/lists/"+*list.Id, h.ownerHost(), h.ownerToken(),
		gen.ListUpdate{Active: ptr(false)})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp, _ = h.req(http.MethodGet, "/public/"+slug, h.ownerHost(), "", nil)
	assert.Equal(t, http.StatusGone, resp.StatusCode)
}

// TestReserverIdentityIsHidden is the critical privacy check: with a real
// reservation carrying a giver name+email, neither the owner item view nor the
// public view leaks that identity — only the availability state changes.
func TestReserverIdentityIsHidden(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	list := h.createList("Party")
	item := h.createItem(*list.Id, "Blender", 1)

	// Seed a reservation directly (the reserve endpoint is #6); it carries a giver
	// identity that must never surface on any response.
	const giverName, giverEmail = "SecretSantaPerson", "santa@example.com"
	_, err := h.store.ForTenant(h.tenant).Reservations().Create(ctx, storage.Reservation{
		ItemID:     *item.Id,
		GiverName:  ptr(giverName),
		GiverEmail: ptr(giverEmail),
		Quantity:   1,
		TokenHash:  "seed-hash",
	})
	require.NoError(t, err)

	// Owner item view: availability flips to reserved, identity absent.
	resp, body := h.req(http.MethodGet, "/api/v1/lists/"+*list.Id+"/items", h.ownerHost(), h.ownerToken(), nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	page := decode[gen.ItemPage](t, body)
	require.Len(t, *page.Items, 1)
	assert.Equal(t, gen.Reserved, *(*page.Items)[0].Availability)
	assert.NotContains(t, string(body), giverName)
	assert.NotContains(t, string(body), giverEmail)

	// Public view: same — availability only, no identity.
	resp, body = h.req(http.MethodGet, "/public/"+*list.ShareSlug, h.ownerHost(), "", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, gen.Reserved, *(*decode[gen.PublicList](t, body).Items)[0].Availability)
	assert.NotContains(t, string(body), giverName)
	assert.NotContains(t, string(body), giverEmail)
}

func TestOutOfScopeOpsReturn501(t *testing.T) {
	h := newHarness(t)
	list := h.createList("L")
	item := h.createItem(*list.Id, "I", 1)

	// Reserve endpoint (public, #6) is stubbed.
	resp, body := h.req(http.MethodPost,
		"/public/"+*list.ShareSlug+"/items/"+*item.Id+"/reservations", h.ownerHost(), "",
		map[string]any{"quantity": 1})
	assert.Equal(t, http.StatusNotImplemented, resp.StatusCode)
	assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))

	// item-previews (owner, scrape) is stubbed.
	resp, _ = h.req(http.MethodPost, "/api/v1/item-previews", h.ownerHost(), h.ownerToken(),
		map[string]any{"url": "https://shop.example/x"})
	assert.Equal(t, http.StatusNotImplemented, resp.StatusCode)

	_ = strings.TrimSpace(string(body))
}
