package api_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/api/gen"
)

// login is a small helper: it logs the seeded credentialed user in and returns the
// access token.
func (h *harness) login(username, password string) string {
	h.t.Helper()
	resp, body := h.req(http.MethodPost, "/api/v1/auth/login", h.ownerHost(), "",
		map[string]any{"username": username, "password": password})
	require.Equal(h.t, http.StatusOK, resp.StatusCode, "login body: %s", body)
	return decode[gen.LoginResponse](h.t, body).AccessToken
}

// TestChangePasswordSuccess: a correct current password changes the credential,
// returns a working session for THIS session, and invalidates the token issued
// before the change (ADR-0011 cut 2, Decision 2).
func TestChangePasswordSuccess(t *testing.T) {
	h := newHarness(t)
	h.seedCredentialedUser("frank", "first-password")
	oldToken := h.login("frank", "first-password")

	// The pre-change token authenticates now.
	resp, _ := h.req(http.MethodGet, "/api/v1/me", h.ownerHost(), oldToken, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Change the password with the correct current one.
	resp, body := h.req(http.MethodPut, "/api/v1/me/password", h.ownerHost(), oldToken,
		map[string]any{"current_password": "first-password", "new_password": "second-password"})
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	newToken := decode[gen.LoginResponse](t, body).AccessToken
	require.NotEmpty(t, newToken)
	assert.NotEqual(t, oldToken, newToken)

	// The returned (acting-session) token works.
	resp, _ = h.req(http.MethodGet, "/api/v1/me", h.ownerHost(), newToken, nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode, "re-issued session authenticates")

	// The token issued before the change is now invalid (every other session dropped).
	resp, _ = h.req(http.MethodGet, "/api/v1/me", h.ownerHost(), oldToken, nil)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "pre-change token invalidated")

	// The new password logs in; the old one no longer does.
	resp, _ = h.req(http.MethodPost, "/api/v1/auth/login", h.ownerHost(), "",
		map[string]any{"username": "frank", "password": "second-password"})
	assert.Equal(t, http.StatusOK, resp.StatusCode, "new password logs in")
	resp, _ = h.req(http.MethodPost, "/api/v1/auth/login", h.ownerHost(), "",
		map[string]any{"username": "frank", "password": "first-password"})
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "old password rejected")
}

// TestChangePasswordWrongCurrent: a wrong current password is rejected with a clear
// reason, and the credential is unchanged.
func TestChangePasswordWrongCurrent(t *testing.T) {
	h := newHarness(t)
	h.seedCredentialedUser("grace", "correct-horse")
	token := h.login("grace", "correct-horse")

	resp, body := h.req(http.MethodPut, "/api/v1/me/password", h.ownerHost(), token,
		map[string]any{"current_password": "wrong-current", "new_password": "brand-new-password"})
	require.Equal(t, http.StatusForbidden, resp.StatusCode, "body: %s", body)
	assert.Contains(t, string(body), "current password is incorrect")

	// The original password still logs in — nothing changed.
	resp, _ = h.req(http.MethodPost, "/api/v1/auth/login", h.ownerHost(), "",
		map[string]any{"username": "grace", "password": "correct-horse"})
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestChangePasswordPolicyRejected: a too-short new password is rejected with a
// clear reason (the shared policy), and the credential is unchanged.
func TestChangePasswordPolicyRejected(t *testing.T) {
	h := newHarness(t)
	h.seedCredentialedUser("heidi", "old-passphrase")
	token := h.login("heidi", "old-passphrase")

	resp, body := h.req(http.MethodPut, "/api/v1/me/password", h.ownerHost(), token,
		map[string]any{"current_password": "old-passphrase", "new_password": "short"})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode, "body: %s", body)
	assert.Contains(t, string(body), "at least")

	// The original password still works.
	resp, _ = h.req(http.MethodPost, "/api/v1/auth/login", h.ownerHost(), "",
		map[string]any{"username": "heidi", "password": "old-passphrase"})
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestChangePasswordMissingFields: an empty body / missing field is a 400.
func TestChangePasswordMissingFields(t *testing.T) {
	h := newHarness(t)
	h.seedCredentialedUser("ivan", "some-password")
	token := h.login("ivan", "some-password")

	resp, _ := h.req(http.MethodPut, "/api/v1/me/password", h.ownerHost(), token,
		map[string]any{"current_password": "some-password"})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// TestChangePasswordRequiresAuth: the endpoint is owner-only.
func TestChangePasswordRequiresAuth(t *testing.T) {
	h := newHarness(t)
	resp, _ := h.req(http.MethodPut, "/api/v1/me/password", h.ownerHost(), "",
		map[string]any{"current_password": "x", "new_password": "brand-new-password"})
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}
