package api_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/api/gen"
	"github.com/yaad-index/yaadegar/internal/auth"
)

// seedAdmin creates the instance superadmin with an argon2id-hashed password —
// the same hash → login round-trip the operator gets via `hash-password`.
func (h *harness) seedAdmin(username, password string) string {
	h.t.Helper()
	hash, err := auth.HashPassword(password)
	require.NoError(h.t, err)
	admin, err := h.store.EnsureAdmin(context.Background(), username, hash)
	require.NoError(h.t, err)
	return admin.ID
}

// anyHost is a host with no configured tenant — the admin surface is not
// tenant-scoped, so /admin works regardless of Host.
const anyHost = "admin.example.test"

// TestAdminSurfaceDisabledByDefault: with no superadmin configured, the whole
// /admin surface reports 404.
func TestAdminSurfaceDisabledByDefault(t *testing.T) {
	h := newHarness(t) // adminEnabled = false

	resp, _ := h.req(http.MethodPost, "/admin/auth/login", anyHost, "",
		map[string]any{"username": "root", "password": "x"})
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	resp, _ = h.req(http.MethodGet, "/admin/me", anyHost, "", nil)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// TestAdminLoginAndMe: with the surface enabled, the superadmin logs in and the
// token authenticates /admin/me — the hash-password → login round-trip.
func TestAdminLoginAndMe(t *testing.T) {
	h := newHarnessOpt(t, true)
	h.seedAdmin("root", "sup3r-secret")

	resp, body := h.req(http.MethodPost, "/admin/auth/login", anyHost, "",
		map[string]any{"username": "root", "password": "sup3r-secret"})
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	lr := decode[gen.LoginResponse](t, body)
	require.NotEmpty(t, lr.AccessToken)

	resp, body = h.req(http.MethodGet, "/admin/me", anyHost, lr.AccessToken, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	me := decode[gen.AdminIdentity](t, body)
	assert.Equal(t, "root", *me.Username)

	// Wrong password → 401.
	resp, _ = h.req(http.MethodPost, "/admin/auth/login", anyHost, "",
		map[string]any{"username": "root", "password": "wrong"})
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	// No token on a protected admin endpoint → 401.
	resp, _ = h.req(http.MethodGet, "/admin/me", anyHost, "", nil)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

// TestAdminOwnerBoundary is the crux security test: the owner/admin boundary holds
// in BOTH directions, each by an independent check.
func TestAdminOwnerBoundary(t *testing.T) {
	h := newHarnessOpt(t, true)
	adminID := h.seedAdmin("root", "sup3r-secret")
	adminTok := h.adminTokenFor(adminID)

	// (1) An owner token is rejected on /admin by the role check (403).
	resp, _ := h.req(http.MethodGet, "/admin/me", anyHost, h.ownerToken(), nil)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode, "owner token must not reach /admin")

	// (2) A superadmin token is rejected on the owner surface — the role=owner
	// assertion AND the sentinel-tid tenant-match both fail.
	resp, _ = h.req(http.MethodGet, "/api/v1/me", h.ownerHost(), adminTok, nil)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "superadmin token must not satisfy owner auth")

	// The superadmin token still works on its own surface (sanity).
	resp, _ = h.req(http.MethodGet, "/admin/me", anyHost, adminTok, nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
