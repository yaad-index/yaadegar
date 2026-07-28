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

// TestListOwnerAuthorization: an authenticated user who is NOT an owner of a list
// (but is in the same tenant) is refused every owner-management op with 403, while
// the actual owner still succeeds. The public/giver surface stays ungated (checked
// at the end) — a non-owner can still view the shared list anonymously.
func TestListOwnerAuthorization(t *testing.T) {
	h := newHarness(t)
	list := h.createList("Alice's list")
	item := h.createItem(*list.Id, "Gift", 1)

	// A second authenticated user in the same tenant, who owns nothing.
	other, err := h.store.ForTenant(h.tenant).Users().Create(context.Background(), storage.User{Name: "Mallory"})
	require.NoError(t, err)
	otherTok := h.tokenFor(other.ID, h.tenant.ID)
	host := h.ownerHost()

	listPath := "/api/v1/lists/" + *list.Id
	itemsPath := listPath + "/items"
	itemPath := "/api/v1/items/" + *item.Id

	// Every owner-surface op on someone else's list → 403.
	forbidden := []struct {
		method, path string
		body         any
	}{
		{http.MethodGet, listPath, nil},
		{http.MethodPatch, listPath, gen.ListUpdate{Title: ptr("hijack")}},
		{http.MethodGet, itemsPath, nil},
		{http.MethodPost, itemsPath, gen.ItemCreate{Name: "sneak"}},
		{http.MethodPatch, itemPath, gen.ItemUpdate{Name: ptr("rename")}},
		{http.MethodDelete, itemPath, nil},
		{http.MethodDelete, listPath, nil},
	}
	for _, c := range forbidden {
		resp, body := h.req(c.method, c.path, host, otherTok, c.body)
		assert.Equal(t, http.StatusForbidden, resp.StatusCode, "%s %s: %s", c.method, c.path, body)
	}

	// The real owner is unaffected.
	resp, _ := h.req(http.MethodGet, listPath, host, h.ownerToken(), nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// A nonexistent list is 404 (not 403) even for the owner — existence vs. ownership.
	resp, _ = h.req(http.MethodGet, "/api/v1/lists/does-not-exist", host, h.ownerToken(), nil)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	// Guardrail: the public/giver surface is NOT ownership-gated — the non-owner
	// (indeed anyone, unauthenticated) can still view the shared list by slug.
	resp, _ = h.req(http.MethodGet, "/public/"+*list.ShareSlug, host, "", nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
