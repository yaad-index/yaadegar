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

// adminUserURL is the admin user-management path for a tenant.
func adminUsersPath(tenantID string) string { return "/admin/tenants/" + tenantID + "/users" }

func TestAdminListTenantsAndUsers(t *testing.T) {
	h := newHarness(t)
	admin := h.seedAdmin()
	adminTok := h.adminToken(admin)

	resp, body := h.req(http.MethodGet, "/admin/tenants", anyHost, adminTok, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	tp := decode[gen.AdminTenantPage](t, body)
	assert.GreaterOrEqual(t, tp.Total, 1)
	found := false
	for _, tn := range tp.Items {
		if tn.Id == h.tenant.ID {
			found = true
		}
	}
	assert.True(t, found, "the seeded tenant should be listed")

	resp, body = h.req(http.MethodGet, adminUsersPath(h.tenant.ID), anyHost, adminTok, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	up := decode[gen.AdminUserPage](t, body)
	require.GreaterOrEqual(t, up.Total, 1)
	// The harness owner defaults to the owner role (migration default) and holds no
	// admin capability; the seeded admin surfaces is_admin=true (ADR-0010).
	var owner, adminUser *gen.AdminUser
	for i := range up.Items {
		switch up.Items[i].Id {
		case h.owner.ID:
			owner = &up.Items[i]
		case admin.ID:
			adminUser = &up.Items[i]
		}
	}
	require.NotNil(t, owner)
	assert.Equal(t, gen.Owner, owner.Role)
	assert.False(t, owner.Banned)
	assert.False(t, owner.IsAdmin, "a plain owner is not an admin")
	require.NotNil(t, adminUser)
	assert.True(t, adminUser.IsAdmin, "the granted account surfaces is_admin")
}

func TestAdminCreateUser(t *testing.T) {
	h := newHarness(t)
	adminTok := h.adminToken(h.seedAdmin())

	// Create a giver by email (no password).
	resp, body := h.req(http.MethodPost, adminUsersPath(h.tenant.ID), anyHost, adminTok,
		map[string]any{"email": "gwen@example.test", "role": "giver"})
	require.Equal(t, http.StatusCreated, resp.StatusCode, "body: %s", body)
	u := decode[gen.AdminUser](t, body)
	assert.Equal(t, gen.Giver, u.Role)
	assert.Equal(t, "gwen@example.test", u.Email)
	assert.False(t, u.Banned)

	// Duplicate email in the same tenant → 409.
	resp, _ = h.req(http.MethodPost, adminUsersPath(h.tenant.ID), anyHost, adminTok,
		map[string]any{"email": "gwen@example.test", "role": "owner"})
	assert.Equal(t, http.StatusConflict, resp.StatusCode)

	// Bad role → 400; unknown tenant → 404.
	resp, _ = h.req(http.MethodPost, adminUsersPath(h.tenant.ID), anyHost, adminTok,
		map[string]any{"email": "x@example.test", "role": "wizard"})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	resp, _ = h.req(http.MethodPost, adminUsersPath("no-such-tenant"), anyHost, adminTok,
		map[string]any{"email": "y@example.test", "role": "giver"})
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestAdminUpdateUser_RoleAndBan(t *testing.T) {
	h := newHarness(t)
	adminTok := h.adminToken(h.seedAdmin())
	ctx := context.Background()

	giver, err := h.store.ForTenant(h.tenant).Users().Create(ctx, storage.User{
		Name: "Gil", Email: "gil@example.test", Role: storage.RoleGiver,
	})
	require.NoError(t, err)
	userURL := adminUsersPath(h.tenant.ID) + "/" + giver.ID

	// Promote giver → owner.
	resp, body := h.req(http.MethodPatch, userURL, anyHost, adminTok, map[string]any{"role": "owner"})
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	assert.Equal(t, gen.Owner, decode[gen.AdminUser](t, body).Role)

	// Ban, then unban.
	resp, body = h.req(http.MethodPatch, userURL, anyHost, adminTok, map[string]any{"banned": true})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.True(t, decode[gen.AdminUser](t, body).Banned)
	resp, body = h.req(http.MethodPatch, userURL, anyHost, adminTok, map[string]any{"banned": false})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.False(t, decode[gen.AdminUser](t, body).Banned)

	// Empty body → 400.
	resp, _ = h.req(http.MethodPatch, userURL, anyHost, adminTok, map[string]any{})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// Demotion of an owner who still owns lists is rejected with 409 and an actionable
// message naming the count (the change-role precondition, ADR-0009 Cut 1).
func TestAdminUpdateUser_DemotionBlockedByOwnedLists(t *testing.T) {
	h := newHarness(t)
	adminTok := h.adminToken(h.seedAdmin())
	ctx := context.Background()

	_, err := h.store.ForTenant(h.tenant).Lists().Create(ctx, storage.List{Title: "Birthday"}, h.owner.ID)
	require.NoError(t, err)

	userURL := adminUsersPath(h.tenant.ID) + "/" + h.owner.ID
	resp, body := h.req(http.MethodPatch, userURL, anyHost, adminTok, map[string]any{"role": "giver"})
	require.Equal(t, http.StatusConflict, resp.StatusCode)
	assert.Contains(t, string(body), "owns")

	// Still an owner (the demotion did not take effect).
	reloaded, err := h.store.ForTenant(h.tenant).Users().Get(ctx, h.owner.ID)
	require.NoError(t, err)
	assert.Equal(t, storage.RoleOwner, reloaded.Role)
}

// A banned account is rejected both at the owner middleware (an existing token) and
// at password login (issue time) — ADR-0009 ban enforcement.
func TestBanEnforcement(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	// An existing owner token stops working once the account is banned (middleware).
	tok := h.ownerToken()
	resp, _ := h.req(http.MethodGet, "/api/v1/me", h.ownerHost(), tok, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, h.store.ForTenant(h.tenant).Users().SetBanned(ctx, h.owner.ID, true))
	resp, _ = h.req(http.MethodGet, "/api/v1/me", h.ownerHost(), tok, nil)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "banned owner token must be rejected")

	// A banned credentialed user cannot log in (issue time).
	pw := "cred-user-pw-123"
	hash, err := auth.HashPassword(pw)
	require.NoError(t, err)
	uname := "banned@example.test"
	u, err := h.store.ForTenant(h.tenant).Users().Create(ctx, storage.User{
		Name: uname, Email: uname, Username: &uname, PasswordHash: hash,
	})
	require.NoError(t, err)
	// Sanity: login works before the ban.
	resp, _ = h.req(http.MethodPost, "/api/v1/auth/login", h.ownerHost(), "",
		map[string]any{"username": uname, "password": pw})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, h.store.ForTenant(h.tenant).Users().SetBanned(ctx, u.ID, true))
	resp, _ = h.req(http.MethodPost, "/api/v1/auth/login", h.ownerHost(), "",
		map[string]any{"username": uname, "password": pw})
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "banned user must not log in")
}
