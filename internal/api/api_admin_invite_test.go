package api_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/api/gen"
	"github.com/yaad-index/yaadegar/internal/storage"
)

// TestAdminCreateUserSendsInvite: creating a user by email emails a single-use
// set-password/invite link, and that link establishes the first credential and
// auto-logs the person in — the unified onboarding path (ADR-0012 Decision 6 / cut 1b).
func TestAdminCreateUserSendsInvite(t *testing.T) {
	h := newHarness(t)
	adminTok := h.adminToken(h.seedAdmin())

	resp, body := h.req(http.MethodPost, adminUsersPath(h.tenant.ID), anyHost, adminTok,
		map[string]any{"email": "ivy@example.test", "role": "owner"})
	require.Equal(t, http.StatusCreated, resp.StatusCode, "body: %s", body)

	// An invite email lands (sent async), addressed to the new account.
	require.Eventually(t, func() bool { return len(h.email.Sent()) == 1 }, time.Second, 10*time.Millisecond)
	msg := h.email.Sent()[0]
	assert.Equal(t, "ivy@example.test", msg.To)
	raw := resetTokenFromEmail(t, msg.Body)

	// The invite establishes a first password and auto-logs-in.
	resp, body = h.req(http.MethodPost, "/api/v1/auth/password-reset/confirm", h.ownerHost(), "",
		map[string]any{"token": raw, "new_password": "chosen-password"})
	require.Equal(t, http.StatusOK, resp.StatusCode, "confirm body: %s", body)
	sessionTok := decode[gen.LoginResponse](t, body).AccessToken
	resp, _ = h.req(http.MethodGet, "/api/v1/me", h.ownerHost(), sessionTok, nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode, "the auto-login session authenticates")

	// And the person can now log in with the password they set.
	resp, _ = h.req(http.MethodPost, "/api/v1/auth/login", h.ownerHost(), "",
		map[string]any{"username": "ivy@example.test", "password": "chosen-password"})
	assert.Equal(t, http.StatusOK, resp.StatusCode, "the established password logs in")
}

// TestNoPasswordAccountEstablishesViaForgotPassword: the widened resettable() guard
// (ADR-0012 Decision 2 / cut 1b) lets a no-password account get a link from the public
// forgot-password path and establish a first password — the request path serves both
// "set first" and "reset".
func TestNoPasswordAccountEstablishesViaForgotPassword(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	// A no-password account (empty hash), as an admin invite or OAuth account would be.
	uname := "jo@example.test"
	_, err := h.store.ForTenant(h.tenant).Users().Create(ctx, storage.User{
		Name: uname, Email: uname, Username: ptr(uname), Role: storage.RoleOwner,
	})
	require.NoError(t, err)

	// Forgot-password on that email sends a link (it would not, before the widening).
	resp, _ := h.req(http.MethodPost, "/api/v1/auth/password-reset/request", h.ownerHost(), "",
		map[string]any{"identifier": uname})
	require.Equal(t, http.StatusAccepted, resp.StatusCode)
	require.Eventually(t, func() bool { return len(h.email.Sent()) == 1 }, time.Second, 10*time.Millisecond)
	raw := resetTokenFromEmail(t, h.email.Sent()[0].Body)

	// Establish the first password + auto-login, then log in with it.
	resp, body := h.req(http.MethodPost, "/api/v1/auth/password-reset/confirm", h.ownerHost(), "",
		map[string]any{"token": raw, "new_password": "first-real-password"})
	require.Equal(t, http.StatusOK, resp.StatusCode, "confirm body: %s", body)
	resp, _ = h.req(http.MethodPost, "/api/v1/auth/login", h.ownerHost(), "",
		map[string]any{"username": uname, "password": "first-real-password"})
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestEstablishPasswordActivatesPendingAccount: setting a password through the emailed
// link also clears a pending account's activation (ADR-0012 cut 1b) — the link proves
// email ownership. Without it, the account would auto-login once but then be unable to
// log in again (login rejects pending).
func TestEstablishPasswordActivatesPendingAccount(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	uname := "lee@example.test"
	u, err := h.store.ForTenant(h.tenant).Users().Create(ctx, storage.User{
		Name: uname, Email: uname, Username: ptr(uname), Role: storage.RoleOwner,
		Status: storage.UserStatusPending,
	})
	require.NoError(t, err)

	resp, _ := h.req(http.MethodPost, "/api/v1/auth/password-reset/request", h.ownerHost(), "",
		map[string]any{"identifier": uname})
	require.Equal(t, http.StatusAccepted, resp.StatusCode)
	require.Eventually(t, func() bool { return len(h.email.Sent()) == 1 }, time.Second, 10*time.Millisecond)
	raw := resetTokenFromEmail(t, h.email.Sent()[0].Body)

	resp, body := h.req(http.MethodPost, "/api/v1/auth/password-reset/confirm", h.ownerHost(), "",
		map[string]any{"token": raw, "new_password": "now-i-have-one"})
	require.Equal(t, http.StatusOK, resp.StatusCode, "confirm body: %s", body)

	// The account is active: it can log in with the new password (a still-pending
	// account would be rejected by the login gate).
	resp, _ = h.req(http.MethodPost, "/api/v1/auth/login", h.ownerHost(), "",
		map[string]any{"username": uname, "password": "now-i-have-one"})
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	reloaded, err := h.store.ForTenant(h.tenant).Users().Get(ctx, u.ID)
	require.NoError(t, err)
	assert.Equal(t, storage.UserStatusActive, reloaded.Status)
}

// TestResettableExcludesBanned: a banned account — even with a deliverable email —
// receives no reset/establish link. Widening the guard to cover no-password accounts
// must not weaken the ban exclusion.
func TestResettableExcludesBanned(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	uname := "kim@example.test"
	u, err := h.store.ForTenant(h.tenant).Users().Create(ctx, storage.User{
		Name: uname, Email: uname, Username: ptr(uname), Role: storage.RoleGiver,
	})
	require.NoError(t, err)
	require.NoError(t, h.store.ForTenant(h.tenant).Users().SetBanned(ctx, u.ID, true))

	resp, _ := h.req(http.MethodPost, "/api/v1/auth/password-reset/request", h.ownerHost(), "",
		map[string]any{"identifier": uname})
	require.Equal(t, http.StatusAccepted, resp.StatusCode, "still enumeration-safe 202")
	// No email is ever sent for a banned account.
	assert.Never(t, func() bool { return len(h.email.Sent()) > 0 }, 200*time.Millisecond, 20*time.Millisecond)
}
