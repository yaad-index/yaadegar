package api_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/api/gen"
	"github.com/yaad-index/yaadegar/internal/auth"
	"github.com/yaad-index/yaadegar/internal/storage"
)

// #185: the signed-in account edits its own display name via PUT /api/v1/me/profile.
// The name defaults to the email at creation; a set value persists and is reflected
// by /me, a blank value falls back to the account email, and the endpoint is
// authenticated.

// seedNamedUser seeds a credentialed user whose display name defaults to the email,
// mirroring how accounts are created in production (name = email at creation).
func (h *harness) seedNamedUser(username, userEmail, password string) storage.User {
	h.t.Helper()
	hash, err := auth.HashPassword(password)
	require.NoError(h.t, err)
	u, err := h.store.ForTenant(h.tenant).Users().Create(context.Background(), storage.User{
		Name: userEmail, Email: userEmail, Username: ptr(username), PasswordHash: hash,
	})
	require.NoError(h.t, err)
	return u
}

func TestUpdateProfileSetsDisplayName(t *testing.T) {
	h := newHarness(t)
	h.seedNamedUser("grace", "grace@example.com", "pw-grace")
	token := h.login("grace", "pw-grace")

	// It defaults to the email at creation.
	resp, body := h.req(http.MethodGet, "/api/v1/me", h.ownerHost(), token, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	assert.Equal(t, "grace@example.com", *decode[gen.User](t, body).Name)

	// Set a display name; the surrounding whitespace is trimmed.
	resp, body = h.req(http.MethodPut, "/api/v1/me/profile", h.ownerHost(), token,
		map[string]any{"name": "  Grace Hopper  "})
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	assert.Equal(t, "Grace Hopper", *decode[gen.User](t, body).Name, "returned user carries the new name")

	// It persists — a fresh /me reflects it.
	resp, body = h.req(http.MethodGet, "/api/v1/me", h.ownerHost(), token, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "Grace Hopper", *decode[gen.User](t, body).Name)
}

func TestUpdateProfileBlankFallsBackToEmail(t *testing.T) {
	h := newHarness(t)
	h.seedNamedUser("heidi", "heidi@example.com", "pw-heidi")
	token := h.login("heidi", "pw-heidi")

	// First set a custom name, then clear it.
	resp, _ := h.req(http.MethodPut, "/api/v1/me/profile", h.ownerHost(), token,
		map[string]any{"name": "Heidi"})
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// A blank (whitespace-only) name falls back to the account email.
	resp, body := h.req(http.MethodPut, "/api/v1/me/profile", h.ownerHost(), token,
		map[string]any{"name": "   "})
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	assert.Equal(t, "heidi@example.com", *decode[gen.User](t, body).Name, "blank name → email")
}

func TestUpdateProfileTooLongRejected(t *testing.T) {
	h := newHarness(t)
	h.seedNamedUser("ivan", "ivan@example.com", "pw-ivan")
	token := h.login("ivan", "pw-ivan")

	// 201 chars exceeds the 200-char display-name cap.
	resp, body := h.req(http.MethodPut, "/api/v1/me/profile", h.ownerHost(), token,
		map[string]any{"name": strings.Repeat("x", 201)})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "over-long name rejected: %s", body)
}

func TestUpdateProfileRequiresAuth(t *testing.T) {
	h := newHarness(t)
	resp, _ := h.req(http.MethodPut, "/api/v1/me/profile", h.ownerHost(), "",
		map[string]any{"name": "Nobody"})
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "unauthenticated update refused")
}
