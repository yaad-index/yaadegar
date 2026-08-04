package api_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/api/gen"
	"github.com/yaad-index/yaadegar/internal/auth"
	"github.com/yaad-index/yaadegar/internal/storage"
	"github.com/yaad-index/yaadegar/internal/token"
)

// roleFromToken decodes a session JWT and returns its role claim, for asserting that
// a session-issuing path stamped the account's real tenant role (#163).
func (h *harness) roleFromToken(tok string) auth.Role {
	h.t.Helper()
	p, err := h.authSvc.Issuer().Validate(tok)
	require.NoError(h.t, err)
	return p.Role
}

// tokenWithRole mints a session JWT with an arbitrary role claim, for the guardrail
// test that a stale/forged token role cannot grant owner access (authz reads the
// stored role, not the claim).
func (h *harness) tokenWithRole(userID, tenantID string, role auth.Role, credentialVersion int) string {
	h.t.Helper()
	tok, err := h.authSvc.Issuer().Issue(auth.Principal{
		UserID: userID, TenantID: tenantID, Role: role, CredentialVersion: credentialVersion,
	})
	require.NoError(h.t, err)
	return tok
}

// confirmResetToken seeds a live reset token for user and confirms it, returning the
// auto-login session token — exercising the shared establish/confirm session path.
func (h *harness) confirmResetToken(userID, newPassword string) string {
	h.t.Helper()
	raw, hash, err := token.New()
	require.NoError(h.t, err)
	_, err = h.store.ForTenant(h.tenant).PasswordResetTokens().Create(context.Background(),
		storage.PasswordResetToken{UserID: userID, TokenHash: hash, ExpiresAt: testClockStart.Add(time.Hour)})
	require.NoError(h.t, err)
	resp, body := h.req(http.MethodPost, "/api/v1/auth/password-reset/confirm", h.ownerHost(), "",
		map[string]any{"token": raw, "new_password": newPassword})
	require.Equal(h.t, http.StatusOK, resp.StatusCode, "reset body: %s", body)
	return decode[gen.LoginResponse](h.t, body).AccessToken
}

// TestSessionTokenCarriesRealRole: every session-issuing path stamps the account's
// actual tenant role on the JWT (#163) — no longer a hardcoded owner.
func TestSessionTokenCarriesRealRole(t *testing.T) {
	t.Run("login owner", func(t *testing.T) {
		h := newHarness(t)
		h.seedOwnerWithEmail("owner1", "owner1@example.com", "long-enough-pass")
		assert.Equal(t, auth.RoleOwner, h.roleFromToken(h.login("owner1", "long-enough-pass")))
	})

	t.Run("login giver", func(t *testing.T) {
		h := newHarness(t)
		h.seedGiver("giver1", "long-enough-pass")
		assert.Equal(t, auth.RoleGiver, h.roleFromToken(h.login("giver1", "long-enough-pass")))
	})

	t.Run("register+verify giver", func(t *testing.T) {
		h := newHarnessRegistration(t, storage.RegistrationGiversOnly)
		resp, _ := h.req(http.MethodPost, "/api/v1/auth/register", h.ownerHost(), "",
			map[string]any{"email": "newbie@example.com", "password": "long-enough-pass", "captcha_token": ""})
		require.Equal(t, http.StatusAccepted, resp.StatusCode)
		require.Eventually(t, func() bool { return len(h.email.Sent()) == 1 }, time.Second, 10*time.Millisecond)
		raw := resetTokenFromEmail(t, h.email.Sent()[0].Body)
		resp, body := h.req(http.MethodPost, "/api/v1/auth/register/verify", h.ownerHost(), "",
			map[string]any{"token": raw})
		require.Equal(t, http.StatusOK, resp.StatusCode, "verify body: %s", body)
		tok := decode[gen.LoginResponse](t, body).AccessToken
		assert.Equal(t, auth.RoleGiver, h.roleFromToken(tok), "a self-registered giver's verify session says giver")
	})

	t.Run("reset auto-login preserves owner role", func(t *testing.T) {
		h := newHarness(t)
		owner := h.seedOwnerWithEmail("owner2", "owner2@example.com", "long-enough-pass")
		assert.Equal(t, auth.RoleOwner, h.roleFromToken(h.confirmResetToken(owner.ID, "second-long-pass")))
	})

	t.Run("reset auto-login preserves giver role", func(t *testing.T) {
		h := newHarness(t)
		giver := h.seedGiver("giver2", "long-enough-pass")
		assert.Equal(t, auth.RoleGiver, h.roleFromToken(h.confirmResetToken(giver.ID, "second-long-pass")))
	})

	t.Run("change-password re-issue preserves giver role", func(t *testing.T) {
		h := newHarness(t)
		h.seedGiver("giver3", "long-enough-pass")
		tok := h.login("giver3", "long-enough-pass")
		resp, body := h.req(http.MethodPut, "/api/v1/me/password", h.ownerHost(), tok,
			map[string]any{"current_password": "long-enough-pass", "new_password": "second-long-pass"})
		require.Equal(t, http.StatusOK, resp.StatusCode, "change-password body: %s", body)
		reissued := decode[gen.LoginResponse](t, body).AccessToken
		assert.Equal(t, auth.RoleGiver, h.roleFromToken(reissued))
	})
}

// TestStaleOwnerRoleTokenForGiverStill403: the guardrail. A giver holding a token that
// (wrongly) claims the owner role — a pre-#163 stale token, or a forged claim — is
// still refused every owner-only endpoint, because authorization reads the STORED
// tenant role, never the token claim. This proves authz did not move onto the JWT.
func TestStaleOwnerRoleTokenForGiverStill403(t *testing.T) {
	h := newHarness(t)
	giver := h.seedGiver("giver", "long-enough-pass")
	// A fresh account is at credential_version 1, so this token passes the cver gate;
	// only its role claim is a lie.
	staleOwnerTok := h.tokenWithRole(giver.ID, h.tenant.ID, auth.RoleOwner, giver.CredentialVersion)

	// It is admitted by requireOwner (valid signature/tenant/cver) …
	resp, _ := h.req(http.MethodGet, "/api/v1/me", h.ownerHost(), staleOwnerTok, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, "the token is a valid session")

	// … but the owner-only surface still refuses it via the stored-role gate.
	resp, body := h.req(http.MethodPost, "/api/v1/lists", h.ownerHost(), staleOwnerTok,
		gen.ListCreate{Title: "Nope"})
	assert.Equal(t, http.StatusForbidden, resp.StatusCode,
		"a giver's owner-claiming token must still 403 on owner-only, body: %s", body)
}

// TestUnrecognizedRoleTokenRejected: requireOwner fails closed on a token whose role
// is neither owner nor giver (defense-in-depth on the authenticated gate).
func TestUnrecognizedRoleTokenRejected(t *testing.T) {
	h := newHarness(t)
	tok := h.tokenWithRole(h.owner.ID, h.tenant.ID, auth.Role("superuser"), h.owner.CredentialVersion)
	resp, _ := h.req(http.MethodGet, "/api/v1/me", h.ownerHost(), tok, nil)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "an unrecognized role is rejected")
}
