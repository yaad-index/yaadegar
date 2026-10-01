package api_test

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/api"
	"github.com/yaad-index/yaadegar/internal/auth"
	"github.com/yaad-index/yaadegar/internal/clock"
	"github.com/yaad-index/yaadegar/internal/storage"
	"github.com/yaad-index/yaadegar/internal/token"
)

// newHarnessPAT builds a harness with its own token and login limiters, so a test
// can see which one counted an attempt.
func newHarnessPAT(t *testing.T, patMax int) (*harness, *auth.InMemoryLimiter, *auth.InMemoryLimiter) {
	t.Helper()
	clk := clock.NewFake(testClockStart)
	patLim := auth.NewInMemoryLimiter(patMax, time.Hour, clk)
	loginLim := auth.NewInMemoryLimiter(patMax, time.Hour, clk)
	h := newHarnessFull(t, loginLim, false, "", captchaConfig{}, "", 0, nil, 0, nil,
		func(o *api.Options) { o.PATLimiter = patLim })
	return h, patLim, loginLim
}

// mintPAT stores a personal access token for userID and returns its raw value.
func (h *harness) mintPAT(userID string, expiresAt *time.Time) (string, storage.AccessToken) {
	h.t.Helper()
	raw, hash, err := token.NewPAT()
	require.NoError(h.t, err)
	stored, err := h.store.ForTenant(h.tenant).AccessTokens().Create(context.Background(), storage.AccessToken{
		UserID: userID, Name: "script", TokenHash: hash, Last4: raw[len(raw)-4:], ExpiresAt: expiresAt,
	}, 20, h.clk.Now())
	require.NoError(h.t, err)
	return raw, stored
}

func (h *harness) patStatus(path, host, raw string) int {
	h.t.Helper()
	resp, _ := h.req(http.MethodGet, path, host, raw, nil)
	return resp.StatusCode
}

func TestAPersonalAccessTokenAuthenticatesTheOwnerSurface(t *testing.T) {
	h, _, _ := newHarnessPAT(t, 5)
	raw, _ := h.mintPAT(h.owner.ID, nil)
	assert.True(t, strings.HasPrefix(raw, token.PATPrefix))
	resp, body := h.req(http.MethodGet, "/api/v1/me", h.ownerHost(), raw, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	assert.Contains(t, string(body), h.owner.ID)
}

// /admin accepts sessions only (ADR-0016 §2), even for an admin's token.
func TestAPersonalAccessTokenIsRejectedOnAdmin(t *testing.T) {
	h, patLim, _ := newHarnessPAT(t, 1)
	admin := h.seedAdmin()
	raw, _ := h.mintPAT(admin.ID, nil)
	resp, body := h.req(http.MethodGet, "/admin/tenants", anyHost, raw, nil)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	assert.Contains(t, string(body), "not accepted here")
	assert.Equal(t, http.StatusOK, h.patStatus("/api/v1/me", h.ownerHost(), raw), "the same token still works on the owner surface")
	assert.True(t, patLim.Allow("pat-ip:"), "a refusal on /admin is not a failed authentication")
}

func TestARevokedExpiredOrBannedTokenIsRejected(t *testing.T) {
	h, _, _ := newHarnessPAT(t, 0)
	ctx := context.Background()

	raw, stored := h.mintPAT(h.owner.ID, nil)
	_, err := h.store.ForTenant(h.tenant).AccessTokens().Revoke(ctx, h.owner.ID, stored.ID, h.clk.Now())
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, h.patStatus("/api/v1/me", h.ownerHost(), raw), "revoked")

	at := h.clk.Now().Add(time.Hour)
	raw, _ = h.mintPAT(h.owner.ID, &at)
	assert.Equal(t, http.StatusOK, h.patStatus("/api/v1/me", h.ownerHost(), raw), "before expiry")
	h.clk.Advance(time.Hour)
	assert.Equal(t, http.StatusUnauthorized, h.patStatus("/api/v1/me", h.ownerHost(), raw), "at expiry")

	raw, _ = h.mintPAT(h.owner.ID, nil)
	require.NoError(t, h.store.ForTenant(h.tenant).Users().SetBanned(ctx, h.owner.ID, true))
	resp, body := h.req(http.MethodGet, "/api/v1/me", h.ownerHost(), raw, nil)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	assert.Contains(t, string(body), "suspended")
}

// A token is found only through its own tenant's host (ADR-0016 §2).
func TestATokenIsRejectedOnAnotherTenantsHost(t *testing.T) {
	h, _, _ := newHarnessPAT(t, 0)
	_, err := h.store.CreateTenant(context.Background(), storage.Tenant{Subdomain: "bob"})
	require.NoError(t, err)
	raw, _ := h.mintPAT(h.owner.ID, nil)
	assert.Equal(t, http.StatusUnauthorized, h.patStatus("/api/v1/me", "bob."+baseDomain, raw))
}

// An ordinary password change bumps the credential version, which ends sessions
// but not tokens (ADR-0016 §4).
func TestAPasswordChangeLeavesTokensValid(t *testing.T) {
	h, _, _ := newHarnessPAT(t, 0)
	u := h.seedCredentialedUser("frank", "first-password")
	session := h.login("frank", "first-password")
	raw, _ := h.mintPAT(u.ID, nil)

	resp, body := h.req(http.MethodPut, "/api/v1/me/password", h.ownerHost(), session,
		map[string]any{"current_password": "first-password", "new_password": "second-password"})
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	assert.Equal(t, http.StatusUnauthorized, h.patStatus("/api/v1/me", h.ownerHost(), session), "the old session ends")
	assert.Equal(t, http.StatusOK, h.patStatus("/api/v1/me", h.ownerHost(), raw), "the token does not")
}

// A token never operates on the account's credentials (ADR-0016 §2).
func TestATokenCannotChangeThePassword(t *testing.T) {
	h, _, _ := newHarnessPAT(t, 0)
	u := h.seedCredentialedUser("frank", "first-password")
	raw, _ := h.mintPAT(u.ID, nil)
	resp, body := h.req(http.MethodPut, "/api/v1/me/password", h.ownerHost(), raw,
		map[string]any{"current_password": "first-password", "new_password": "second-password"})
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	assert.Contains(t, string(body), "personal access token")
	assert.NotEmpty(t, h.login("frank", "first-password"), "the password is unchanged")
}

// Failed token authentications trip their own limiter, before the lookup, and
// leave the login limiter alone; failed logins do not count against tokens.
func TestFailedTokenAuthenticationsHaveTheirOwnLimiter(t *testing.T) {
	h, _, loginLim := newHarnessPAT(t, 2)
	raw, _ := h.mintPAT(h.owner.ID, nil)

	for range 2 {
		resp, _ := h.req(http.MethodPost, "/api/v1/auth/login", h.ownerHost(), "", map[string]any{"username": "nobody", "password": "x"})
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	}
	assert.False(t, loginLim.Allow("ip:"), "two failed logins lock the login limiter")
	assert.Equal(t, http.StatusOK, h.patStatus("/api/v1/me", h.ownerHost(), raw), "and leave tokens alone")

	h2, _, loginLim2 := newHarnessPAT(t, 2)
	raw2, _ := h2.mintPAT(h2.owner.ID, nil)
	assert.Equal(t, http.StatusUnauthorized, h2.patStatus("/api/v1/me", h2.ownerHost(), token.PATPrefix+"guess-one"))
	assert.Equal(t, http.StatusUnauthorized, h2.patStatus("/api/v1/me", h2.ownerHost(), token.PATPrefix+"guess-two"))
	assert.Equal(t, http.StatusTooManyRequests, h2.patStatus("/api/v1/me", h2.ownerHost(), token.PATPrefix+"guess-three"))
	assert.Equal(t, http.StatusTooManyRequests, h2.patStatus("/api/v1/me", h2.ownerHost(), raw2),
		"the limiter is checked before the lookup, so it bounds lookups for a locked-out address (§6 as written)")
	assert.True(t, loginLim2.Allow("ip:"), "failed token authentications leave the login limiter alone")
	assert.Equal(t, http.StatusOK, h2.patStatus("/api/v1/me", h2.ownerHost(), h2.ownerToken()), "sessions are unaffected")
}

// A revoked token is a failed authentication; a valid one is not counted.
func TestOnlyFailedTokenAuthenticationsCount(t *testing.T) {
	h, patLim, _ := newHarnessPAT(t, 1)
	raw, stored := h.mintPAT(h.owner.ID, nil)
	for range 3 {
		require.Equal(t, http.StatusOK, h.patStatus("/api/v1/me", h.ownerHost(), raw))
	}
	assert.True(t, patLim.Allow("pat-ip:"), "successful token requests are not counted")
	_, err := h.store.ForTenant(h.tenant).AccessTokens().Revoke(context.Background(), h.owner.ID, stored.ID, h.clk.Now())
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, h.patStatus("/api/v1/me", h.ownerHost(), raw))
	assert.False(t, patLim.Allow("pat-ip:"), "a revoked token counts as a failure")
}

// Last-used is written at most once per five minutes per token (ADR-0016 §3).
func TestLastUsedIsWrittenAtMostEveryFiveMinutes(t *testing.T) {
	h, _, _ := newHarnessPAT(t, 0)
	raw, stored := h.mintPAT(h.owner.ID, nil)
	lastUsed := func() *time.Time {
		list, err := h.store.ForTenant(h.tenant).AccessTokens().ListByUser(context.Background(), h.owner.ID)
		require.NoError(t, err)
		for _, tk := range list {
			if tk.ID == stored.ID {
				return tk.LastUsedAt
			}
		}
		t.Fatal("token not listed")
		return nil
	}
	assert.Nil(t, lastUsed())

	first := h.clk.Now()
	require.Equal(t, http.StatusOK, h.patStatus("/api/v1/me", h.ownerHost(), raw))
	require.NotNil(t, lastUsed())
	assert.True(t, lastUsed().Equal(first))

	h.clk.Advance(4 * time.Minute)
	require.Equal(t, http.StatusOK, h.patStatus("/api/v1/me", h.ownerHost(), raw))
	assert.True(t, lastUsed().Equal(first), "within five minutes, not written")

	h.clk.Advance(2 * time.Minute)
	require.Equal(t, http.StatusOK, h.patStatus("/api/v1/me", h.ownerHost(), raw))
	assert.True(t, lastUsed().Equal(h.clk.Now()), "after five minutes, written")
}

// patVia sends a token request through peer with an X-Forwarded-For value.
func (h *harness) patVia(peer, xff, raw string) int {
	h.t.Helper()
	req, err := http.NewRequest(http.MethodGet, "http://"+h.ownerHost()+"/api/v1/me", nil)
	require.NoError(h.t, err)
	req.Host = h.ownerHost()
	req.RemoteAddr = peer + ":40000"
	req.Header.Set("Authorization", "Bearer "+raw)
	req.Header.Set("X-Forwarded-For", xff)
	rec := &responseRecorder{header: http.Header{}, body: &bytes.Buffer{}}
	h.h.ServeHTTP(rec, req)
	return rec.result().StatusCode
}

// The token limiter is keyed on the client address resolved through the trusted
// proxies (#464), so one client's failures lock out that client only.
func TestTheTokenLimiterIsPerClient(t *testing.T) {
	proxies, err := api.ParseTrustedProxies([]string{"10.0.0.1"})
	require.NoError(t, err)
	patLim := auth.NewInMemoryLimiter(1, time.Hour, clock.NewFake(testClockStart))
	h := newHarnessFull(t, nil, false, "", captchaConfig{}, "", 0, nil, 0, proxies,
		func(o *api.Options) { o.PATLimiter = patLim })
	raw, _ := h.mintPAT(h.owner.ID, nil)

	assert.Equal(t, http.StatusUnauthorized, h.patVia("10.0.0.1", "203.0.113.5", token.PATPrefix+"guess"))
	assert.Equal(t, http.StatusTooManyRequests, h.patVia("10.0.0.1", "203.0.113.5", raw), "client A is locked out")
	assert.Equal(t, http.StatusOK, h.patVia("10.0.0.1", "203.0.113.9", raw), "client B behind the same proxy is not")
}
