package api_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/storage"
)

// meStatus issues GET /api/v1/me with a Host, optional X-Forwarded-Host, and
// optional bearer token, returning the status. 404 = tenant unresolved, 401 =
// resolved but auth failed / tenant mismatch, 200 = resolved + authed for that
// tenant — so the status reveals which tenant the request routed to.
func (h *harness) meStatus(host, xfh, token string) int {
	headers := map[string]string{}
	if xfh != "" {
		headers["X-Forwarded-Host"] = xfh
	}
	if token != "" {
		headers["Authorization"] = "Bearer " + token
	}
	resp, _ := h.reqH(http.MethodGet, "/api/v1/me", host, headers, nil)
	return resp.StatusCode
}

// seedBob adds a second tenant so cross-tenant routing is observable (alice is
// seeded by the harness).
func (h *harness) seedBob() {
	_, err := h.store.CreateTenant(context.Background(), storage.Tenant{Subdomain: "bob"})
	require.NoError(h.t, err)
}

// TestForwardedHostTrust is the load-bearing security test for the proxy trust
// gate (ADR-0004 §7): X-Forwarded-Host is honored ONLY when trust is enabled.
func TestForwardedHostTrust(t *testing.T) {
	aliceHost := "alice." + baseDomain
	bobHost := "bob." + baseDomain

	// (2) THE load-bearing case: with trust OFF (the default), a client-set
	// X-Forwarded-Host must be ignored — the tenant resolves from Host only, so an
	// attacker cannot route into another tenant.
	t.Run("trust OFF (default): X-Forwarded-Host ignored", func(t *testing.T) {
		h := newHarness(t)
		h.seedBob()
		// Host=alice + XFH=bob → resolves ALICE (alice token matches) → 200. If XFH
		// were honored it would route to bob and the alice token would be a 401.
		assert.Equal(t, http.StatusOK, h.meStatus(aliceHost, bobHost, h.ownerToken()))
	})

	// (1) With trust ON, X-Forwarded-Host takes precedence over Host.
	t.Run("trust ON: X-Forwarded-Host honored", func(t *testing.T) {
		h := newHarnessTrusted(t)
		h.seedBob()
		// Host=bob (wrong) + XFH=alice → resolves ALICE (token matches) → 200. Without
		// honoring XFH it would resolve bob and the alice token would be a 401.
		assert.Equal(t, http.StatusOK, h.meStatus(bobHost, aliceHost, h.ownerToken()))
	})

	// (3) With trust ON but no X-Forwarded-Host present, fall back to Host.
	t.Run("trust ON + no X-Forwarded-Host: falls back to Host", func(t *testing.T) {
		h := newHarnessTrusted(t)
		assert.Equal(t, http.StatusOK, h.meStatus(aliceHost, "", h.ownerToken()))
	})
}
