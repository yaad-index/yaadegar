package api_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/api/gen"
	"github.com/yaad-index/yaadegar/internal/auth"
	"github.com/yaad-index/yaadegar/internal/storage"
	"github.com/yaad-index/yaadegar/internal/token"
)

// seedOwnerWithEmail creates a credentialed owner that also has a deliverable email,
// so it is eligible for a password reset.
func (h *harness) seedOwnerWithEmail(username, email, password string) storage.User {
	h.t.Helper()
	hash, err := auth.HashPassword(password)
	require.NoError(h.t, err)
	u, err := h.store.ForTenant(h.tenant).Users().Create(context.Background(), storage.User{
		Name: username, Username: ptr(username), Email: email, PasswordHash: hash,
	})
	require.NoError(h.t, err)
	return u
}

// resetTokenFromEmail extracts the raw reset token from the most recent reset email.
func resetTokenFromEmail(t *testing.T, body string) string {
	t.Helper()
	i := strings.Index(body, "token=")
	require.GreaterOrEqual(t, i, 0, "email should carry a reset link: %s", body)
	rest := body[i+len("token="):]
	if end := strings.IndexAny(rest, " \n\r"); end >= 0 {
		rest = rest[:end]
	}
	require.NotEmpty(t, rest)
	return rest
}

// TestPasswordResetRequestEnumerationSafe: request returns an identical 202 for a
// real account, its email form, and an unknown identifier; an email goes out only
// for the real account.
func TestPasswordResetRequestEnumerationSafe(t *testing.T) {
	h := newHarness(t)
	h.seedOwnerWithEmail("erin", "erin@example.com", "first-password")

	for _, id := range []string{"erin", "erin@example.com", "ghost"} {
		resp, body := h.req(http.MethodPost, "/api/v1/auth/password-reset/request", h.ownerHost(), "",
			map[string]any{"identifier": id})
		require.Equal(t, http.StatusAccepted, resp.StatusCode, "identifier %q", id)
		assert.Empty(t, body, "202 body is empty for %q", id)
	}

	// Exactly one email — to the real account, addressed by either identifier form,
	// and none for the unknown one. (Send is async, so allow it to land.)
	assert.Eventually(t, func() bool { return len(h.email.Sent()) == 2 }, time.Second, 10*time.Millisecond)
	for _, m := range h.email.Sent() {
		assert.Equal(t, "erin@example.com", m.To)
	}
}

// TestPasswordResetFullFlow: the emailed token completes a reset, auto-logs-in, and
// invalidates a session issued before the reset.
func TestPasswordResetFullFlow(t *testing.T) {
	h := newHarness(t)
	h.seedOwnerWithEmail("frank", "frank@example.com", "first-password")

	// A session issued before the reset, to prove it gets invalidated.
	oldToken := h.login("frank", "first-password")
	resp, _ := h.req(http.MethodGet, "/api/v1/me", h.ownerHost(), oldToken, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Request the reset and pull the token out of the email.
	resp, _ = h.req(http.MethodPost, "/api/v1/auth/password-reset/request", h.ownerHost(), "",
		map[string]any{"identifier": "frank@example.com"})
	require.Equal(t, http.StatusAccepted, resp.StatusCode)
	var raw string
	require.Eventually(t, func() bool { return len(h.email.Sent()) == 1 }, time.Second, 10*time.Millisecond)
	raw = resetTokenFromEmail(t, h.email.Sent()[0].Body)

	// Confirm with a new password → 200 + an auto-login session.
	resp, body := h.req(http.MethodPost, "/api/v1/auth/password-reset/confirm", h.ownerHost(), "",
		map[string]any{"token": raw, "new_password": "second-password"})
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	newToken := decode[gen.LoginResponse](t, body).AccessToken
	require.NotEmpty(t, newToken)

	// The auto-login session works; the pre-reset session is now invalid.
	resp, _ = h.req(http.MethodGet, "/api/v1/me", h.ownerHost(), newToken, nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode, "auto-login session authenticates")
	resp, _ = h.req(http.MethodGet, "/api/v1/me", h.ownerHost(), oldToken, nil)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "pre-reset session invalidated")

	// The new password logs in; the old one no longer does.
	resp, _ = h.req(http.MethodPost, "/api/v1/auth/login", h.ownerHost(), "",
		map[string]any{"username": "frank", "password": "second-password"})
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp, _ = h.req(http.MethodPost, "/api/v1/auth/login", h.ownerHost(), "",
		map[string]any{"username": "frank", "password": "first-password"})
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	// The token is single-use: a replay is rejected.
	resp, _ = h.req(http.MethodPost, "/api/v1/auth/password-reset/confirm", h.ownerHost(), "",
		map[string]any{"token": raw, "new_password": "third-password"})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "used token is rejected")
}

// TestPasswordResetConfirmInvalidTokens: an unknown token and an expired token both
// yield the same generic 400.
func TestPasswordResetConfirmInvalidTokens(t *testing.T) {
	h := newHarness(t)
	user := h.seedOwnerWithEmail("grace", "grace@example.com", "old-password")

	// Unknown token.
	resp, _ := h.req(http.MethodPost, "/api/v1/auth/password-reset/confirm", h.ownerHost(), "",
		map[string]any{"token": "not-a-real-token", "new_password": "brand-new-pass"})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	// Expired token: seed one with an expiry before the harness clock.
	raw, hash, err := token.New()
	require.NoError(t, err)
	_, err = h.store.ForTenant(h.tenant).PasswordResetTokens().Create(context.Background(),
		storage.PasswordResetToken{UserID: user.ID, TokenHash: hash, ExpiresAt: testClockStart.Add(-time.Hour)})
	require.NoError(t, err)
	resp, _ = h.req(http.MethodPost, "/api/v1/auth/password-reset/confirm", h.ownerHost(), "",
		map[string]any{"token": raw, "new_password": "brand-new-pass"})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "expired token is rejected")
}

// TestPasswordResetConfirmPolicyDoesNotBurnToken: a too-short new password is
// rejected AND leaves the token usable, so the user can retry with a valid one.
func TestPasswordResetConfirmPolicyDoesNotBurnToken(t *testing.T) {
	h := newHarness(t)
	user := h.seedOwnerWithEmail("heidi", "heidi@example.com", "old-password")
	raw, hash, err := token.New()
	require.NoError(t, err)
	_, err = h.store.ForTenant(h.tenant).PasswordResetTokens().Create(context.Background(),
		storage.PasswordResetToken{UserID: user.ID, TokenHash: hash, ExpiresAt: testClockStart.Add(time.Hour)})
	require.NoError(t, err)

	// Too short → 400, and the token is NOT consumed.
	resp, _ := h.req(http.MethodPost, "/api/v1/auth/password-reset/confirm", h.ownerHost(), "",
		map[string]any{"token": raw, "new_password": "short"})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)

	// Retry with a policy-conformant password → 200 (the link still worked).
	resp, body := h.req(http.MethodPost, "/api/v1/auth/password-reset/confirm", h.ownerHost(), "",
		map[string]any{"token": raw, "new_password": "long-enough-pass"})
	assert.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
}

// TestPasswordResetRequestValidation: a missing identifier is a 400.
func TestPasswordResetRequestValidation(t *testing.T) {
	h := newHarness(t)
	resp, _ := h.req(http.MethodPost, "/api/v1/auth/password-reset/request", h.ownerHost(), "",
		map[string]any{})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}
