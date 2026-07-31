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

func decodeMethods(t *testing.T, rec interface{ Result() *http.Response }) gen.LoginMethods {
	t.Helper()
	var m gen.LoginMethods
	require.NoError(t, json.NewDecoder(rec.Result().Body).Decode(&m))
	return m
}

// On the tenant's subdomain, both methods are available (client configured +
// toggle on), and login_url is the canonical subdomain login page.
func TestAuthMethods_SubdomainBothEnabled(t *testing.T) {
	o := newOAuthHarness(t, "alice@example.com", true)
	rec := o.getOn(o.tenantHost(), "/api/v1/auth/methods", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	m := decodeMethods(t, rec)
	assert.True(t, m.Password)
	assert.True(t, m.Google)
	assert.Equal(t, "https://alice.example.test/login", m.LoginUrl)
}

// Google is off when the tenant toggle is off, even with a configured client.
func TestAuthMethods_GoogleOffWhenToggleOff(t *testing.T) {
	o := newOAuthHarness(t, "alice@example.com", false) // toggle off
	rec := o.getOn(o.tenantHost(), "/api/v1/auth/methods", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	m := decodeMethods(t, rec)
	assert.True(t, m.Password)
	assert.False(t, m.Google)
}

// On a verified custom domain (public-giver-only), every login affordance is off
// and login_url points at the canonical subdomain so the frontend can redirect.
func TestAuthMethods_CustomDomainAllOff(t *testing.T) {
	o := newOAuthHarness(t, "alice@example.com", true)
	// Attach a verified custom domain to the tenant.
	_, err := o.store.ForTenant(o.tenant).Domains().Create(context.Background(), storage.Domain{
		Hostname: "shop.example.com",
		Verified: true,
	})
	require.NoError(t, err)

	rec := o.getOn("shop.example.com", "/api/v1/auth/methods", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	m := decodeMethods(t, rec)
	assert.False(t, m.Password, "password login is main-domain-only")
	assert.False(t, m.Google, "google login is main-domain-only")
	assert.Equal(t, "https://alice.example.test/login", m.LoginUrl)
}

// The OAuth /start endpoint refuses to begin a login on a custom domain (defense
// in depth behind the methods-gated frontend affordance).
func TestOAuthStart_RejectsCustomDomain(t *testing.T) {
	o := newOAuthHarness(t, "alice@example.com", true)
	_, err := o.store.ForTenant(o.tenant).Domains().Create(context.Background(), storage.Domain{
		Hostname: "shop.example.com",
		Verified: true,
	})
	require.NoError(t, err)

	rec := o.getOn("shop.example.com", "/api/v1/auth/oauth/google/start?tenant=shop.example.com", nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}
