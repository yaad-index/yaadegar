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

// seedCredentialedUser creates an owner with a username + argon2id password hash.
func (h *harness) seedCredentialedUser(username, password string) storage.User {
	h.t.Helper()
	hash, err := auth.HashPassword(password)
	require.NoError(h.t, err)
	u, err := h.store.ForTenant(h.tenant).Users().Create(context.Background(), storage.User{
		Name: username, Username: ptr(username), PasswordHash: hash,
	})
	require.NoError(h.t, err)
	return u
}

// TestLoginIssuesUsableToken: a correct password yields a JWT that authenticates a
// subsequent owner-surface request.
func TestLoginIssuesUsableToken(t *testing.T) {
	h := newHarness(t)
	h.seedCredentialedUser("carol", "hunter2!")

	resp, body := h.req(http.MethodPost, "/api/v1/auth/login", h.ownerHost(), "",
		map[string]any{"username": "carol", "password": "hunter2!"})
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	lr := decode[gen.LoginResponse](t, body)
	require.NotEmpty(t, lr.AccessToken)
	assert.Equal(t, gen.Bearer, lr.TokenType)
	assert.Positive(t, lr.ExpiresIn)

	// The token authenticates the owner surface.
	resp, body = h.req(http.MethodGet, "/api/v1/me", h.ownerHost(), lr.AccessToken, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
}

// TestLoginRejectsBadCredentials: wrong password, unknown user, and a
// credential-less user all return 401.
func TestLoginRejectsBadCredentials(t *testing.T) {
	h := newHarness(t)
	h.seedCredentialedUser("carol", "hunter2!")

	cases := []map[string]any{
		{"username": "carol", "password": "wrong"},     // wrong password
		{"username": "nobody", "password": "hunter2!"}, // unknown user
	}
	for _, in := range cases {
		resp, _ := h.req(http.MethodPost, "/api/v1/auth/login", h.ownerHost(), "", in)
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "input: %v", in)
	}

	// The seeded owner (h.owner) has no username/password → can't be logged into.
	resp, _ := h.req(http.MethodPost, "/api/v1/auth/login", h.ownerHost(), "",
		map[string]any{"username": "Alice", "password": "whatever"})
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

// TestOwnerSurfaceRequiresValidToken: no token and a garbage token are both 401.
func TestOwnerSurfaceRequiresValidToken(t *testing.T) {
	h := newHarness(t)

	resp, _ := h.req(http.MethodGet, "/api/v1/me", h.ownerHost(), "", nil)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "missing token")

	resp, _ = h.req(http.MethodGet, "/api/v1/me", h.ownerHost(), "not-a-jwt", nil)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "garbage token")
}

// TestTenantMatchInvariant is the cross-tenant-replay guard (ADR-0005 §5): a token
// minted for one tenant is rejected on another tenant's host.
func TestTenantMatchInvariant(t *testing.T) {
	h := newHarness(t)
	// A second tenant so its host resolves (else the mismatch would 404 on routing).
	_, err := h.store.CreateTenant(context.Background(), storage.Tenant{Subdomain: "bob"})
	require.NoError(t, err)

	// Alice's valid token, presented on Bob's host → rejected (tid mismatch), even
	// though the token itself is otherwise valid.
	resp, _ := h.req(http.MethodGet, "/api/v1/me", "bob."+baseDomain, h.ownerToken(), nil)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	// Sanity: the same token works on Alice's own host.
	resp, _ = h.req(http.MethodGet, "/api/v1/me", h.ownerHost(), h.ownerToken(), nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestStaleCredentialVersionRejected: a token whose cver claim no longer matches the
// user's stored credential_version is rejected on the owner surface (ADR-0011).
func TestStaleCredentialVersionRejected(t *testing.T) {
	h := newHarness(t)

	// The seeded owner is at stored version 1. A token pinning a different version is
	// a stale session and must be rejected, even though it is otherwise valid.
	stale := h.tokenForVersion(h.owner.ID, h.tenant.ID, 99)
	resp, _ := h.req(http.MethodGet, "/api/v1/me", h.ownerHost(), stale, nil)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	// Sanity: the matching-version token works.
	resp, _ = h.req(http.MethodGet, "/api/v1/me", h.ownerHost(), h.ownerToken(), nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestPasswordChangeInvalidatesToken: a password mutation bumps the stored
// credential_version, so a previously-issued session token stops authenticating
// while a freshly-issued one works (ADR-0011 — the core invalidation guarantee).
func TestPasswordChangeInvalidatesToken(t *testing.T) {
	h := newHarness(t)
	user := h.seedCredentialedUser("erin", "first-password")

	// Log in → a live session token (cver = 1).
	resp, body := h.req(http.MethodPost, "/api/v1/auth/login", h.ownerHost(), "",
		map[string]any{"username": "erin", "password": "first-password"})
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	oldToken := decode[gen.LoginResponse](t, body).AccessToken

	// It authenticates now.
	resp, _ = h.req(http.MethodGet, "/api/v1/me", h.ownerHost(), oldToken, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// A password mutation (the set-password funnel's storage half) bumps the version.
	newHash, err := auth.HashNewPassword("second-password")
	require.NoError(t, err)
	require.NoError(t, h.store.ForTenant(h.tenant).Users().SetPasswordHash(context.Background(), user.ID, newHash))

	// The old token is now invalid — the stored version moved past its cver.
	resp, _ = h.req(http.MethodGet, "/api/v1/me", h.ownerHost(), oldToken, nil)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	// A fresh login picks up the new version and authenticates.
	resp, body = h.req(http.MethodPost, "/api/v1/auth/login", h.ownerHost(), "",
		map[string]any{"username": "erin", "password": "second-password"})
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	newToken := decode[gen.LoginResponse](t, body).AccessToken
	resp, _ = h.req(http.MethodGet, "/api/v1/me", h.ownerHost(), newToken, nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestLoginIsUnauthenticated: the login endpoint itself needs no token (it is the
// way to get one).
func TestLoginIsUnauthenticated(t *testing.T) {
	h := newHarness(t)
	h.seedCredentialedUser("dave", "passphrase-1")
	resp, _ := h.req(http.MethodPost, "/api/v1/auth/login", h.ownerHost(), "",
		map[string]any{"username": "dave", "password": "passphrase-1"})
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
