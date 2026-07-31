package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/api/gen"
	"github.com/yaad-index/yaadegar/internal/storage"
)

func parseSettings(t *testing.T, body []byte) gen.TenantSettings {
	t.Helper()
	var s gen.TenantSettings
	require.NoError(t, json.Unmarshal(body, &s))
	return s
}

func TestTenantSettings_GetDefaults(t *testing.T) {
	h := newHarness(t)
	resp, body := h.req(http.MethodGet, "/api/v1/settings", h.ownerHost(), h.ownerToken(), nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	s := parseSettings(t, body)
	assert.False(t, s.OauthGoogleEnabled, "toggle defaults off")
	assert.False(t, s.GoogleClientConfigured, "no Google client configured in this harness")
}

func TestTenantSettings_RequiresAuth(t *testing.T) {
	h := newHarness(t)
	resp, _ := h.req(http.MethodGet, "/api/v1/settings", h.ownerHost(), "", nil)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestTenantSettings_PatchTogglesOwnTenant(t *testing.T) {
	h := newHarness(t)

	resp, body := h.req(http.MethodPatch, "/api/v1/settings", h.ownerHost(), h.ownerToken(),
		gen.TenantSettingsUpdate{OauthGoogleEnabled: true})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.True(t, parseSettings(t, body).OauthGoogleEnabled)

	// Persisted: a fresh GET reflects it, and so does the stored tenant.
	_, body2 := h.req(http.MethodGet, "/api/v1/settings", h.ownerHost(), h.ownerToken(), nil)
	assert.True(t, parseSettings(t, body2).OauthGoogleEnabled)

	reloaded, err := h.store.TenantByID(context.Background(), h.tenant.ID)
	require.NoError(t, err)
	assert.True(t, reloaded.OAuthGoogleEnabled)

	// Turning it back off works too.
	_, body3 := h.req(http.MethodPatch, "/api/v1/settings", h.ownerHost(), h.ownerToken(),
		gen.TenantSettingsUpdate{OauthGoogleEnabled: false})
	assert.False(t, parseSettings(t, body3).OauthGoogleEnabled)
}

// The write targets only the authenticated owner's tenant: flipping one tenant's
// toggle leaves another tenant untouched (the tenant comes from the session, not a
// request parameter).
func TestTenantSettings_DoesNotAffectOtherTenant(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	other, err := h.store.CreateTenant(ctx, storage.Tenant{Subdomain: "bob"})
	require.NoError(t, err)

	resp, _ := h.req(http.MethodPatch, "/api/v1/settings", h.ownerHost(), h.ownerToken(),
		gen.TenantSettingsUpdate{OauthGoogleEnabled: true})
	require.Equal(t, http.StatusOK, resp.StatusCode)

	reloaded, err := h.store.TenantByID(ctx, other.ID)
	require.NoError(t, err)
	assert.False(t, reloaded.OAuthGoogleEnabled, "another tenant's toggle is unchanged")
}
