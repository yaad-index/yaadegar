package api_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/api/gen"
	"github.com/yaad-index/yaadegar/internal/auth"
	"github.com/yaad-index/yaadegar/internal/storage"
)

// seedGiver creates an active giver account with a password credential + username,
// mirroring a verified self-registered giver (ADR-0009/0012).
func (h *harness) seedGiver(username, password string) storage.User {
	h.t.Helper()
	hash, err := auth.HashPassword(password)
	require.NoError(h.t, err)
	u, err := h.store.ForTenant(h.tenant).Users().Create(context.Background(), storage.User{
		Name: username, Username: ptr(username), Email: username + "@example.com",
		PasswordHash: hash, Role: storage.RoleGiver, Status: storage.UserStatusActive,
	})
	require.NoError(h.t, err)
	return u
}

// TestGiverRefusedOwnerOnlyEndpoints: a giver's valid session (its token honestly
// carries the giver role, #163, and is admitted by requireOwner) is refused with 403
// on each owner-only endpoint via the stored-role gate. The role gate fires before
// any not-found handling, so even a nonexistent id yields 403 (ADR-0009).
func TestGiverRefusedOwnerOnlyEndpoints(t *testing.T) {
	h := newHarness(t)
	giver := h.seedGiver("giver", "long-enough-pass")
	tok := h.login("giver", "long-enough-pass") // an active giver CAN hold a session

	cases := []struct {
		name   string
		method string
		path   string
		body   any
	}{
		{"createList", http.MethodPost, "/api/v1/lists", gen.ListCreate{Title: "Nope"}},
		{"updateTenantSettings", http.MethodPatch, "/api/v1/settings", gen.TenantSettingsUpdate{OauthGoogleEnabled: true}},
		{"addDomain", http.MethodPost, "/api/v1/domains", map[string]any{"hostname": "gifts.example.com"}},
		{"verifyDomain", http.MethodPost, "/api/v1/domains/does-not-exist/verify", nil},
		{"deleteDomain", http.MethodDelete, "/api/v1/domains/does-not-exist", nil},
		{"importList", http.MethodPost, "/api/v1/lists/does-not-exist/import", nil},
		{"exportList", http.MethodGet, "/api/v1/lists/does-not-exist/export", nil},
		{"previewItem", http.MethodPost, "/api/v1/item-previews", map[string]any{"url": "https://example.com/p"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resp, body := h.req(c.method, c.path, h.ownerHost(), tok, c.body)
			assert.Equal(t, http.StatusForbidden, resp.StatusCode,
				"%s must be 403 for a giver (not 404), body: %s", c.name, body)
		})
	}
	// Sanity: the seeded account really is a giver.
	got, err := h.store.ForTenant(h.tenant).Users().Get(context.Background(), giver.ID)
	require.NoError(t, err)
	assert.Equal(t, storage.RoleGiver, got.Role)
}

// TestGiverAllowedEndpoints: the SAME giver session still reaches its own account
// surface — /me and changing its own password — proving the gate is scoped to the
// owner-only endpoints and does not lock a giver out of self-service (ADR-0012).
func TestGiverAllowedEndpoints(t *testing.T) {
	h := newHarness(t)
	h.seedGiver("giver", "long-enough-pass")
	tok := h.login("giver", "long-enough-pass")

	// /me works.
	resp, _ := h.req(http.MethodGet, "/api/v1/me", h.ownerHost(), tok, nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode, "a giver can read /me")

	// A giver may change its own password.
	resp, body := h.req(http.MethodPut, "/api/v1/me/password", h.ownerHost(), tok,
		map[string]any{"current_password": "long-enough-pass", "new_password": "second-long-pass"})
	assert.Equal(t, http.StatusOK, resp.StatusCode, "a giver can change its own password, body: %s", body)
}

// TestOwnerUnaffectedByRoleGate: a normal owner account is unaffected — CreateList and
// UpdateTenantSettings still succeed (no regression from the giver gate).
func TestOwnerUnaffectedByRoleGate(t *testing.T) {
	h := newHarness(t)
	// The seeded harness owner (h.owner) has the default owner role.
	tok := h.ownerToken()

	resp, body := h.req(http.MethodPost, "/api/v1/lists", h.ownerHost(), tok, gen.ListCreate{Title: "Mine"})
	require.Equal(t, http.StatusCreated, resp.StatusCode, "an owner can create a list, body: %s", body)

	resp, body = h.req(http.MethodPatch, "/api/v1/settings", h.ownerHost(), tok,
		gen.TenantSettingsUpdate{OauthGoogleEnabled: true})
	require.Equal(t, http.StatusOK, resp.StatusCode, "an owner can update tenant settings, body: %s", body)
}
