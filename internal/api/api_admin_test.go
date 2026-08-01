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

// seedAdmin creates an owner carrying the instance-admin capability in the harness
// tenant and returns it (ADR-0010). Admin is a flag on an ordinary owner, not a
// separate identity — so the returned account reaches /admin with an owner session.
func (h *harness) seedAdmin() storage.User {
	h.t.Helper()
	u, err := h.store.ForTenant(h.tenant).Users().Create(context.Background(), storage.User{
		Name: "Root", IsAdmin: true,
	})
	require.NoError(h.t, err)
	return u
}

// anyHost is a host with no configured tenant — the admin surface is not
// tenant-scoped, so /admin works regardless of Host.
const anyHost = "admin.example.test"

// TestAdminSurfaceRequiresCapability: the /admin surface is always mounted, but an
// unauthenticated caller is 401 and a non-admin owner is 403.
func TestAdminSurfaceRequiresCapability(t *testing.T) {
	h := newHarness(t)

	// Unauthenticated → 401.
	resp, _ := h.req(http.MethodGet, "/admin/tenants", anyHost, "", nil)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	// A plain owner (no capability) → 403.
	resp, _ = h.req(http.MethodGet, "/admin/tenants", anyHost, h.ownerToken(), nil)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode, "a non-admin owner must not reach /admin")
}

// TestAdminCapabilityGrantsSurface: an is_admin owner reaches /admin with its
// ordinary owner session, and that same session is only a normal owner on /api/v1.
func TestAdminCapabilityGrantsSurface(t *testing.T) {
	h := newHarness(t)
	adminTok := h.adminToken(h.seedAdmin())

	// The capability opens the admin surface.
	resp, body := h.req(http.MethodGet, "/admin/tenants", anyHost, adminTok, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)

	// The same token is just an owner on /api/v1 for its home tenant (the capability
	// grants nothing here) — a normal owner call succeeds.
	resp, _ = h.req(http.MethodGet, "/api/v1/me", h.ownerHost(), adminTok, nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// And the tenant-match invariant is unchanged: the admin's token does not satisfy
	// another tenant's owner surface.
	resp, _ = h.req(http.MethodGet, "/api/v1/me", "neworg."+baseDomain, adminTok, nil)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode, "unknown tenant host is 404 before auth")
}

// TestAdminRevocationTakesEffectImmediately: clearing the capability locks the
// account out of /admin on the very next request (per-request flag load).
func TestAdminRevocationTakesEffectImmediately(t *testing.T) {
	h := newHarness(t)
	admin := h.seedAdmin()
	tok := h.adminToken(admin)

	resp, _ := h.req(http.MethodGet, "/admin/tenants", anyHost, tok, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	require.NoError(t, h.store.ForTenant(h.tenant).Users().SetAdmin(context.Background(), admin.ID, false))

	resp, _ = h.req(http.MethodGet, "/admin/tenants", anyHost, tok, nil)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode, "revoked capability locks out immediately")
}

// TestAdminBannedRejected: a banned admin is rejected on the admin surface too.
func TestAdminBannedRejected(t *testing.T) {
	h := newHarness(t)
	admin := h.seedAdmin()
	tok := h.adminToken(admin)
	require.NoError(t, h.store.ForTenant(h.tenant).Users().SetBanned(context.Background(), admin.ID, true))

	resp, _ := h.req(http.MethodGet, "/admin/tenants", anyHost, tok, nil)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "a banned admin is rejected")
}

// TestAdminProvisioning: an admin creates a tenant and an owner over HTTP, and the
// created owner logs in end-to-end on the new tenant's host. Also the error matrix
// (409 duplicates, 404 unknown tenant, 403 non-admin owner, 401 unauth).
func TestAdminProvisioning(t *testing.T) {
	h := newHarness(t)
	adminTok := h.adminToken(h.seedAdmin())

	// Create a tenant.
	resp, body := h.req(http.MethodPost, "/admin/tenants", anyHost, adminTok,
		map[string]any{"subdomain": "neworg"})
	require.Equal(t, http.StatusCreated, resp.StatusCode, "body: %s", body)
	ten := decode[gen.Tenant](t, body)
	require.NotEmpty(t, *ten.Id)
	assert.Equal(t, "neworg", *ten.Subdomain)

	// Create an owner in it (password hashed server-side; no hash echoed back).
	resp, body = h.req(http.MethodPost, "/admin/owners", anyHost, adminTok,
		map[string]any{"tenant_id": *ten.Id, "email": "carol@example.test", "password": "ownerpw123"})
	require.Equal(t, http.StatusCreated, resp.StatusCode, "body: %s", body)
	ow := decode[gen.AdminOwner](t, body)
	require.NotEmpty(t, *ow.Id)
	assert.Equal(t, "carol@example.test", *ow.Email)
	assert.NotContains(t, string(body), "argon2", "the password hash must never be returned")

	// End-to-end: the created owner logs in on the new tenant's host (email = username).
	newHost := "neworg." + baseDomain
	resp, body = h.req(http.MethodPost, "/api/v1/auth/login", newHost, "",
		map[string]any{"username": "carol@example.test", "password": "ownerpw123"})
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	lr := decode[gen.LoginResponse](t, body)
	resp, _ = h.req(http.MethodGet, "/api/v1/me", newHost, lr.AccessToken, nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Error matrix.
	resp, _ = h.req(http.MethodPost, "/admin/tenants", anyHost, adminTok, map[string]any{"subdomain": "neworg"})
	assert.Equal(t, http.StatusConflict, resp.StatusCode, "duplicate subdomain")

	resp, _ = h.req(http.MethodPost, "/admin/owners", anyHost, adminTok,
		map[string]any{"tenant_id": *ten.Id, "email": "carol@example.test", "password": "x"})
	assert.Equal(t, http.StatusConflict, resp.StatusCode, "duplicate owner")

	resp, _ = h.req(http.MethodPost, "/admin/owners", anyHost, adminTok,
		map[string]any{"tenant_id": "does-not-exist", "email": "e@x.test", "password": "x"})
	assert.Equal(t, http.StatusNotFound, resp.StatusCode, "unknown tenant")

	resp, _ = h.req(http.MethodPost, "/admin/tenants", anyHost, h.ownerToken(), map[string]any{"subdomain": "x"})
	assert.Equal(t, http.StatusForbidden, resp.StatusCode, "non-admin owner rejected from admin provisioning")

	resp, _ = h.req(http.MethodPost, "/admin/tenants", anyHost, "", map[string]any{"subdomain": "x"})
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "unauth rejected")
}
