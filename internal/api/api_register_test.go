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

// userByEmail resolves a seeded/created account by email, for asserting the
// self-registered account's role and status.
func (h *harness) userByEmail(email string) (storage.User, error) {
	return h.store.ForTenant(h.tenant).Users().ByEmail(context.Background(), email)
}

// TestRegisterDisabledByDefault: with no registration policy (disabled), the
// register endpoint answers 403 and creates nothing.
func TestRegisterDisabledByDefault(t *testing.T) {
	h := newHarness(t)
	resp, _ := h.req(http.MethodPost, "/api/v1/auth/register", h.ownerHost(), "",
		map[string]any{"email": "newbie@example.com", "password": "long-enough-pass", "captcha_token": ""})
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)

	_, err := h.userByEmail("newbie@example.com")
	assert.ErrorIs(t, err, storage.ErrNotFound, "no account created when registration is disabled")
}

// TestRegisterGiversOnlyFullFlow: a new email self-registers a pending giver, the
// verification email lands, and verifying auto-logs-in + activates the account so it
// can log in.
func TestRegisterGiversOnlyFullFlow(t *testing.T) {
	h := newHarnessRegistration(t, storage.RegistrationGiversOnly)

	resp, body := h.req(http.MethodPost, "/api/v1/auth/register", h.ownerHost(), "",
		map[string]any{"email": "newbie@example.com", "password": "long-enough-pass", "captcha_token": ""})
	require.Equal(t, http.StatusAccepted, resp.StatusCode, "body: %s", body)
	assert.Empty(t, body, "202 body is empty")

	// The account is created pending with the giver role.
	require.Eventually(t, func() bool { return len(h.email.Sent()) == 1 }, time.Second, 10*time.Millisecond)
	created, err := h.userByEmail("newbie@example.com")
	require.NoError(t, err)
	assert.Equal(t, storage.RoleGiver, created.Role, "givers_only creates a giver")
	assert.Equal(t, storage.UserStatusPending, created.Status, "the new account is pending")

	// A pending account cannot log in yet (it can log in by email — the username).
	resp, _ = h.req(http.MethodPost, "/api/v1/auth/login", h.ownerHost(), "",
		map[string]any{"username": "newbie@example.com", "password": "long-enough-pass"})
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "pending account cannot log in")

	// Verify with the emailed token → 200 + an auto-login session.
	raw := resetTokenFromEmail(t, h.email.Sent()[0].Body)
	resp, body = h.req(http.MethodPost, "/api/v1/auth/register/verify", h.ownerHost(), "",
		map[string]any{"token": raw})
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	sessionToken := decode[gen.LoginResponse](t, body).AccessToken
	require.NotEmpty(t, sessionToken)

	// The auto-login session authenticates /api/v1/me.
	resp, _ = h.req(http.MethodGet, "/api/v1/me", h.ownerHost(), sessionToken, nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode, "auto-login session authenticates")

	// The account is now active and can log in by email.
	activated, err := h.userByEmail("newbie@example.com")
	require.NoError(t, err)
	assert.Equal(t, storage.UserStatusActive, activated.Status)
	resp, _ = h.req(http.MethodPost, "/api/v1/auth/login", h.ownerHost(), "",
		map[string]any{"username": "newbie@example.com", "password": "long-enough-pass"})
	assert.Equal(t, http.StatusOK, resp.StatusCode, "the verified account can log in")
}

// TestRegisterOwnersAllowedRole: owners_allowed creates an owner account.
func TestRegisterOwnersAllowedRole(t *testing.T) {
	h := newHarnessRegistration(t, storage.RegistrationOwnersAllowed)
	resp, _ := h.req(http.MethodPost, "/api/v1/auth/register", h.ownerHost(), "",
		map[string]any{"email": "founder@example.com", "password": "long-enough-pass", "captcha_token": ""})
	require.Equal(t, http.StatusAccepted, resp.StatusCode)
	require.Eventually(t, func() bool { return len(h.email.Sent()) == 1 }, time.Second, 10*time.Millisecond)
	created, err := h.userByEmail("founder@example.com")
	require.NoError(t, err)
	assert.Equal(t, storage.RoleOwner, created.Role, "owners_allowed creates an owner")
}

// TestRegisterEnumerationSafe: registering an already-existing email still returns
// 202 but creates no second account and sends no verification email.
func TestRegisterEnumerationSafe(t *testing.T) {
	h := newHarnessRegistration(t, storage.RegistrationGiversOnly)
	h.seedOwnerWithEmail("existing", "existing@example.com", "first-password")

	resp, body := h.req(http.MethodPost, "/api/v1/auth/register", h.ownerHost(), "",
		map[string]any{"email": "existing@example.com", "password": "long-enough-pass", "captcha_token": ""})
	require.Equal(t, http.StatusAccepted, resp.StatusCode, "identical 202 for an existing email")
	assert.Empty(t, body)

	// No verification email is ever sent for the existing account. Give any (wrongly
	// dispatched) async send a chance to land, then assert none did.
	assert.Never(t, func() bool { return len(h.email.Sent()) > 0 }, 200*time.Millisecond, 20*time.Millisecond)

	// Still exactly one account with that email (no duplicate pending account), and it
	// is unchanged (still active, still an owner).
	got, err := h.userByEmail("existing@example.com")
	require.NoError(t, err)
	assert.Equal(t, storage.UserStatusActive, got.Status, "the existing account is untouched")
	users, total, err := h.store.ForTenant(h.tenant).Users().List(context.Background(), storage.Page{Limit: 200})
	require.NoError(t, err)
	count := 0
	for _, u := range users {
		if u.Email == "existing@example.com" {
			count++
		}
	}
	assert.Equal(t, 1, count, "no second account for the existing email (total users %d)", total)
}

// TestRegisterTooShortPassword: a too-short password is a 400 and creates nothing.
func TestRegisterTooShortPassword(t *testing.T) {
	h := newHarnessRegistration(t, storage.RegistrationGiversOnly)
	resp, _ := h.req(http.MethodPost, "/api/v1/auth/register", h.ownerHost(), "",
		map[string]any{"email": "shorty@example.com", "password": "short", "captcha_token": ""})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	_, err := h.userByEmail("shorty@example.com")
	assert.ErrorIs(t, err, storage.ErrNotFound, "no account created on a policy failure")
}

// TestRegisterMissingFields: a missing email or password is a 400.
func TestRegisterMissingFields(t *testing.T) {
	h := newHarnessRegistration(t, storage.RegistrationGiversOnly)
	resp, _ := h.req(http.MethodPost, "/api/v1/auth/register", h.ownerHost(), "",
		map[string]any{"password": "long-enough-pass"})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// TestRegisterVerifyInvalidTokens: an unknown token, an expired token, and a used
// token all yield the same generic 400.
func TestRegisterVerifyInvalidTokens(t *testing.T) {
	h := newHarnessRegistration(t, storage.RegistrationGiversOnly)

	// Unknown token.
	resp, _ := h.req(http.MethodPost, "/api/v1/auth/register/verify", h.ownerHost(), "",
		map[string]any{"token": "not-a-real-token"})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	// Missing token.
	resp, _ = h.req(http.MethodPost, "/api/v1/auth/register/verify", h.ownerHost(), "",
		map[string]any{})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	// Expired token: seed a pending account + an already-expired verification token.
	user := h.seedPendingGiver("pending@example.com", "long-enough-pass")
	raw, hash, err := token.New()
	require.NoError(t, err)
	_, err = h.store.ForTenant(h.tenant).EmailVerificationTokens().Create(context.Background(),
		storage.EmailVerificationToken{UserID: user.ID, TokenHash: hash, ExpiresAt: testClockStart.Add(-time.Hour)})
	require.NoError(t, err)
	resp, _ = h.req(http.MethodPost, "/api/v1/auth/register/verify", h.ownerHost(), "",
		map[string]any{"token": raw})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "expired token is rejected")

	// A used token: seed one with used_at set (claimed) and confirm a replay is 400.
	raw2, hash2, err := token.New()
	require.NoError(t, err)
	created, err := h.store.ForTenant(h.tenant).EmailVerificationTokens().Create(context.Background(),
		storage.EmailVerificationToken{UserID: user.ID, TokenHash: hash2, ExpiresAt: testClockStart.Add(time.Hour)})
	require.NoError(t, err)
	_, err = h.store.ForTenant(h.tenant).EmailVerificationTokens().MarkUsed(context.Background(), created.ID, testClockStart)
	require.NoError(t, err)
	resp, _ = h.req(http.MethodPost, "/api/v1/auth/register/verify", h.ownerHost(), "",
		map[string]any{"token": raw2})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "used token is rejected")
}

// TestPendingAccountCannotLogIn: a seeded pending account with a correct password is
// still refused a session (the login pending gate, ADR-0012).
func TestPendingAccountCannotLogIn(t *testing.T) {
	h := newHarness(t)
	h.seedPendingGiver("waiting@example.com", "long-enough-pass")
	resp, _ := h.req(http.MethodPost, "/api/v1/auth/login", h.ownerHost(), "",
		map[string]any{"username": "waiting@example.com", "password": "long-enough-pass"})
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "a pending account cannot log in")
}

// seedPendingGiver creates a pending giver account with a password credential and its
// email as the login handle (mirroring the self-registration insert).
func (h *harness) seedPendingGiver(email, password string) storage.User {
	h.t.Helper()
	hash, err := auth.HashPassword(password)
	require.NoError(h.t, err)
	u, err := h.store.ForTenant(h.tenant).Users().Create(context.Background(), storage.User{
		Name: email, Username: ptr(email), Email: email, PasswordHash: hash,
		Role: storage.RoleGiver, Status: storage.UserStatusPending,
	})
	require.NoError(h.t, err)
	return u
}
