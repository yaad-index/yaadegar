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

// TestLoginIsUnauthenticated: the login endpoint itself needs no token (it is the
// way to get one).
func TestLoginIsUnauthenticated(t *testing.T) {
	h := newHarness(t)
	h.seedCredentialedUser("dave", "passphrase-1")
	resp, _ := h.req(http.MethodPost, "/api/v1/auth/login", h.ownerHost(), "",
		map[string]any{"username": "dave", "password": "passphrase-1"})
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
