package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/api"
	"github.com/yaad-index/yaadegar/internal/api/gen"
	"github.com/yaad-index/yaadegar/internal/auth"
	"github.com/yaad-index/yaadegar/internal/storage"
	"github.com/yaad-index/yaadegar/internal/token"
)

// freshSession mints a session for the seeded owner issued now.
func (h *harness) freshSession() string { return h.ownerToken() }

func (h *harness) createToken(session string, body map[string]any) (*http.Response, []byte) {
	h.t.Helper()
	return h.req(http.MethodPost, "/api/v1/me/tokens", h.ownerHost(), session, body)
}

func never(name string) map[string]any { return map[string]any{"name": name, "never_expires": true} }

func TestCreatingAToken(t *testing.T) {
	h := newHarness(t)
	exp := h.clk.Now().Add(30 * 24 * time.Hour)
	resp, body := h.createToken(h.freshSession(), map[string]any{"name": "  backup script  ", "expires_at": exp})
	require.Equal(t, http.StatusCreated, resp.StatusCode, "body: %s", body)
	created := decode[gen.CreatedAccessToken](t, body)
	assert.True(t, strings.HasPrefix(created.Token, token.PATPrefix))
	assert.Equal(t, "backup script", created.AccessToken.Name, "the name is trimmed")
	assert.Equal(t, created.Token[len(created.Token)-4:], created.AccessToken.Last4)
	require.NotNil(t, created.AccessToken.ExpiresAt)
	assert.True(t, created.AccessToken.ExpiresAt.Equal(exp))
	assert.True(t, created.AccessToken.Active)

	resp, body = h.req(http.MethodGet, "/api/v1/me", h.ownerHost(), created.Token, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, "the new token authenticates: %s", body)

	resp, body = h.createToken(h.freshSession(), never("ci"))
	require.Equal(t, http.StatusCreated, resp.StatusCode, "body: %s", body)
	assert.Nil(t, decode[gen.CreatedAccessToken](t, body).AccessToken.ExpiresAt, "never_expires")
}

// The stored record never holds the raw value, and neither does the list.
func TestTheTokenValueIsShownOnce(t *testing.T) {
	h := newHarness(t)
	_, body := h.createToken(h.freshSession(), never("ci"))
	raw := decode[gen.CreatedAccessToken](t, body).Token

	stored, err := h.store.ForTenant(h.tenant).AccessTokens().ListByUser(context.Background(), h.owner.ID)
	require.NoError(t, err)
	require.Len(t, stored, 1)
	rec, _ := json.Marshal(stored[0])
	assert.NotContains(t, string(rec), raw)
	assert.NotContains(t, string(rec), strings.TrimPrefix(raw, token.PATPrefix))
	assert.Equal(t, token.Hash(raw), stored[0].TokenHash)

	resp, list := h.req(http.MethodGet, "/api/v1/me/tokens", h.ownerHost(), h.freshSession(), nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NotContains(t, string(list), raw)
	assert.NotContains(t, string(list), stored[0].TokenHash)
}

func TestACreateRequestMustBeComplete(t *testing.T) {
	h := newHarness(t)
	past := h.clk.Now().Add(-time.Minute)
	future := h.clk.Now().Add(time.Hour)
	for name, body := range map[string]map[string]any{
		"no expiry choice":   {"name": "x"},
		"both expiries":      {"name": "x", "expires_at": future, "never_expires": true},
		"expiry in the past": {"name": "x", "expires_at": past},
		"never false":        {"name": "x", "never_expires": false},
		"blank name":         {"name": "   ", "never_expires": true},
		"long name":          {"name": strings.Repeat("n", 101), "never_expires": true},
	} {
		resp, b := h.createToken(h.freshSession(), body)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "%s: %s", name, b)
	}
	resp, b := h.createToken(h.freshSession(), map[string]any{"name": strings.Repeat("n", 100), "never_expires": true})
	assert.Equal(t, http.StatusCreated, resp.StatusCode, "a 100-character name: %s", b)
}

// Creation needs a session issued within ten minutes (ADR-0016 §5); a session
// with no issue time is not fresh.
func TestCreationNeedsARecentSignIn(t *testing.T) {
	h := newHarness(t)
	stale := h.freshSession()
	h.clk.Advance(10 * time.Minute)
	resp, b := h.createToken(stale, never("a"))
	require.Equal(t, http.StatusCreated, resp.StatusCode, "exactly ten minutes is still recent: %s", b)
	h.clk.Advance(time.Second)
	resp, b = h.createToken(stale, never("b"))
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	assert.Contains(t, string(b), "recent sign-in")
	resp, _ = h.createToken(h.freshSession(), never("c"))
	assert.Equal(t, http.StatusCreated, resp.StatusCode, "signing in again works")

	noIat := jwt.NewWithClaims(jwt.SigningMethodHS256, auth.Claims{
		TenantID: h.tenant.ID, Role: auth.RoleOwner, CredentialVersion: 1,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: h.owner.ID, Issuer: auth.DefaultIssuer,
			ExpiresAt: jwt.NewNumericDate(h.clk.Now().Add(time.Hour)),
		},
	})
	signed, err := noIat.SignedString([]byte(testJWTSecret))
	require.NoError(t, err)
	resp, _ = h.req(http.MethodGet, "/api/v1/me", h.ownerHost(), signed, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, "the session itself is valid")
	resp, _ = h.createToken(signed, never("d"))
	assert.Equal(t, http.StatusForbidden, resp.StatusCode, "but carries no issue time, so it is not recent")
}

// A fresh sign-in by OAuth is as good as any other (ADR-0016 §5): OAuth-only
// accounts are the ones that need tokens most.
func TestCreationWorksAfterAFreshOAuthSignIn(t *testing.T) {
	o := newOAuthHarness(t, "alice@example.com", true)
	o.mock.next = mockIdentity{sub: "google-sub-1", email: "alice@example.com", emailVerified: true}
	session := o.completeLogin(t, "")
	b, err := json.Marshal(never("ci"))
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "http://"+o.tenantHost()+"/api/v1/me/tokens", bytes.NewReader(b))
	req.Host = o.tenantHost()
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+session.Value)
	rec := httptest.NewRecorder()
	o.h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusCreated, rec.Code, "body: %s", rec.Body.String())
}

// A token never operates on the account's tokens (ADR-0016 §2).
func TestATokenCannotManageTokens(t *testing.T) {
	h := newHarness(t)
	_, body := h.createToken(h.freshSession(), never("ci"))
	created := decode[gen.CreatedAccessToken](t, body)
	raw := created.Token

	for name, c := range map[string]struct{ method, path string }{
		"create": {http.MethodPost, "/api/v1/me/tokens"},
		"list":   {http.MethodGet, "/api/v1/me/tokens"},
		"revoke": {http.MethodDelete, "/api/v1/me/tokens/" + created.AccessToken.Id},
	} {
		var reqBody any
		if c.method == http.MethodPost {
			reqBody = never("successor")
		}
		resp, b := h.req(c.method, c.path, h.ownerHost(), raw, reqBody)
		assert.Equal(t, http.StatusForbidden, resp.StatusCode, "%s: %s", name, b)
		assert.Contains(t, string(b), "personal access token cannot manage tokens", name)
	}
	resp, _ := h.req(http.MethodGet, "/api/v1/me", h.ownerHost(), raw, nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode, "the token was not revoked by its own attempt")
	stored, err := h.store.ForTenant(h.tenant).AccessTokens().ListByUser(context.Background(), h.owner.ID)
	require.NoError(t, err)
	assert.Len(t, stored, 1, "and minted no successor")
}

func TestATwentyFirstActiveTokenIsRefused(t *testing.T) {
	h := newHarness(t)
	var first gen.AccessToken
	for i := range 20 {
		resp, body := h.createToken(h.freshSession(), never("t"))
		require.Equal(t, http.StatusCreated, resp.StatusCode, "token %d: %s", i+1, body)
		if i == 0 {
			first = decode[gen.CreatedAccessToken](t, body).AccessToken
		}
	}
	resp, body := h.createToken(h.freshSession(), never("t21"))
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
	assert.Contains(t, string(body), "20 active tokens")

	resp, _ = h.req(http.MethodDelete, "/api/v1/me/tokens/"+first.Id, h.ownerHost(), h.freshSession(), nil)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	resp, _ = h.createToken(h.freshSession(), never("t21"))
	assert.Equal(t, http.StatusCreated, resp.StatusCode, "succeeds again once one is revoked")
}

func TestListingAndRevokingTokens(t *testing.T) {
	h := newHarness(t)
	_, b1 := h.createToken(h.freshSession(), never("older"))
	h.clk.Advance(time.Minute)
	_, b2 := h.createToken(h.freshSession(), never("newer"))
	older := decode[gen.CreatedAccessToken](t, b1)
	newer := decode[gen.CreatedAccessToken](t, b2)

	resp, _ := h.req(http.MethodDelete, "/api/v1/me/tokens/"+older.AccessToken.Id, h.ownerHost(), h.freshSession(), nil)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	resp, _ = h.req(http.MethodDelete, "/api/v1/me/tokens/"+older.AccessToken.Id, h.ownerHost(), h.freshSession(), nil)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode, "revoking again succeeds")
	resp, _ = h.req(http.MethodGet, "/api/v1/me", h.ownerHost(), older.Token, nil)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "a revoked token stops working")

	h.clk.Advance(time.Hour)
	resp, body := h.req(http.MethodGet, "/api/v1/me/tokens", h.ownerHost(), h.freshSession(), nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	list := decode[[]gen.AccessToken](t, body)
	require.Len(t, list, 2, "revoked tokens are listed too")
	assert.Equal(t, newer.AccessToken.Id, list[0].Id, "newest first")
	assert.True(t, list[0].Active)
	assert.False(t, list[1].Active)
	assert.NotNil(t, list[1].RevokedAt)

	resp, _ = h.req(http.MethodDelete, "/api/v1/me/tokens/nope", h.ownerHost(), h.freshSession(), nil)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	other := h.seedCredentialedUser("bob", "bobs-password")
	resp, _ = h.req(http.MethodDelete, "/api/v1/me/tokens/"+newer.AccessToken.Id, h.ownerHost(), h.tokenFor(other.ID, h.tenant.ID), nil)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode, "another account's token is not found")
	resp, _ = h.req(http.MethodGet, "/api/v1/me", h.ownerHost(), newer.Token, nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode, "and still works")
}

// The public surface reads no bearer credential, so a token is never even
// looked up there.
func TestATokenGrantsNothingOnThePublicSurface(t *testing.T) {
	h := newHarness(t)
	list := h.createList("Birthday")
	_, body := h.createToken(h.freshSession(), never("ci"))
	created := decode[gen.CreatedAccessToken](t, body)
	resp, _ := h.req(http.MethodGet, "/public/"+*list.ShareSlug, h.ownerHost(), created.Token, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	stored, err := h.store.ForTenant(h.tenant).AccessTokens().ByHash(context.Background(), token.Hash(created.Token))
	require.NoError(t, err)
	assert.Nil(t, stored.LastUsedAt, "the public request never authenticated with it")
}

// An ordinary password change leaves tokens valid and reports how many; a
// forgot-password reset revokes them all and says so (ADR-0016 §4).
func TestPasswordChangeKeepsTokensAndResetRevokesThem(t *testing.T) {
	h := newHarness(t)
	u := h.seedOwnerWithEmail("frank", "frank@example.com", "first-password")
	session := h.login("frank", "first-password")
	ts := h.store.ForTenant(h.tenant)
	var last storage.AccessToken
	for range 3 {
		raw, hash, err := token.NewPAT()
		require.NoError(t, err)
		last, err = ts.AccessTokens().Create(context.Background(), storage.AccessToken{UserID: u.ID, Name: "s", TokenHash: hash, Last4: raw[len(raw)-4:]}, 20, h.clk.Now())
		require.NoError(t, err)
	}
	_, err := ts.AccessTokens().Revoke(context.Background(), u.ID, last.ID, h.clk.Now())
	require.NoError(t, err)

	resp, body := h.req(http.MethodPut, "/api/v1/me/password", h.ownerHost(), session,
		map[string]any{"current_password": "first-password", "new_password": "second-password"})
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	assert.Equal(t, 2, *decode[gen.ChangePasswordResponse](t, body).ActiveTokens, "a revoked token is not counted")

	resp, _ = h.req(http.MethodPost, "/api/v1/auth/password-reset/request", h.ownerHost(), "", map[string]any{"identifier": "frank"})
	require.Equal(t, http.StatusAccepted, resp.StatusCode)
	require.Eventually(t, func() bool { return len(h.email.Sent()) == 1 }, time.Second, 10*time.Millisecond)
	raw := resetTokenFromEmail(t, h.email.Sent()[0].Body)
	resp, body = h.req(http.MethodPost, "/api/v1/auth/password-reset/confirm", h.ownerHost(), "",
		map[string]any{"token": raw, "new_password": "third-password"})
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	assert.Equal(t, 2, decode[gen.PasswordResetConfirmResponse](t, body).TokensRevoked, "the already revoked one is not counted again")
	list, err := ts.AccessTokens().ListByUser(context.Background(), u.ID)
	require.NoError(t, err)
	require.Len(t, list, 3)
	for _, tk := range list {
		assert.NotNil(t, tk.RevokedAt)
	}
}

// failingTokenList is a store whose access-token list always fails, for the
// password-change path that reads it after the change has committed.
type failingTokenList struct{ storage.Store }

func (s failingTokenList) ForTenant(t storage.Tenant) storage.TenantStore {
	return failingTokenListTenant{s.Store.ForTenant(t)}
}

type failingTokenListTenant struct{ storage.TenantStore }

func (t failingTokenListTenant) AccessTokens() storage.AccessTokenRepo {
	return failingTokenListRepo{t.TenantStore.AccessTokens()}
}

type failingTokenListRepo struct{ storage.AccessTokenRepo }

func (failingTokenListRepo) ListByUser(context.Context, string) ([]storage.AccessToken, error) {
	return nil, errors.New("the database went away")
}

// The token count after a password change is informational: when it cannot be
// read, the change still succeeds and the caller still gets its new session.
func TestAPasswordChangeSucceedsWhenTheTokenCountFails(t *testing.T) {
	h := newHarness(t)
	h.seedCredentialedUser("frank", "first-password")
	session := h.login("frank", "first-password")
	handler := api.NewHandler(failingTokenList{h.store}, api.Options{
		BaseDomain: baseDomain, Logger: slog.New(slog.DiscardHandler), Clock: h.clk, Auth: h.authSvc,
	})

	b, err := json.Marshal(map[string]any{"current_password": "first-password", "new_password": "second-password"})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPut, "http://"+h.ownerHost()+"/api/v1/me/password", bytes.NewReader(b))
	req.Host = h.ownerHost()
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+session)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	got := decode[gen.ChangePasswordResponse](t, rec.Body.Bytes())
	assert.NotEmpty(t, got.AccessToken, "the new session is returned")
	assert.Nil(t, got.ActiveTokens, "the count is left out")
	assert.NotContains(t, rec.Body.String(), "active_tokens")
	resp, _ := h.req(http.MethodGet, "/api/v1/me", h.ownerHost(), got.AccessToken, nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode, "and it works")
}
