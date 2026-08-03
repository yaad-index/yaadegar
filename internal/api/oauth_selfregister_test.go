package api_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/storage"
)

// TestOAuth_SelfRegister_GiverUnderGiversOnly: an unknown Google-verified email
// self-registers a giver account under givers_only (ADR-0012 cut 2). The account is
// created active with no password, and — because the OAuth session carries a giver on
// its user row — it is still refused on the owner-only surface (cut 1a gate holds).
func TestOAuth_SelfRegister_GiverUnderGiversOnly(t *testing.T) {
	o := newOAuthHarnessWithPolicy(t, "alice@example.com", true, storage.RegistrationGiversOnly)
	o.mock.next = mockIdentity{sub: "google-new-giver", email: "newgiver@example.com", emailVerified: true}

	session := o.completeLogin(t, "/dashboard")
	principal, err := o.authIssuerValidate(session.Value)
	require.NoError(t, err)
	assert.NotEqual(t, o.owner.ID, principal.UserID, "a new account, not the seeded owner")
	assert.Equal(t, o.tenant.ID, principal.TenantID)

	// The provisioned account: giver role, active, no password (empty hash).
	ctx := context.Background()
	u, err := o.store.ForTenant(o.tenant).Users().ByEmail(ctx, "newgiver@example.com")
	require.NoError(t, err)
	assert.Equal(t, storage.RoleGiver, u.Role)
	assert.Equal(t, storage.UserStatusActive, u.Status)
	assert.Empty(t, u.PasswordHash, "OAuth self-register creates a no-password account")

	// The provider subject is linked so the next login resolves straight through.
	oi, err := o.store.ForTenant(o.tenant).OAuthIdentities().
		ByProviderSubject(ctx, storage.OAuthProviderGoogle, "google-new-giver")
	require.NoError(t, err)
	assert.Equal(t, u.ID, oi.UserID)

	// Owner-only surface still refuses the giver (cut 1a requireOwnerRole gate). The
	// export handler checks the role before the list lookup, so any listId yields 403.
	rec := o.getAuth(o.tenantHost(), "/api/v1/lists/any-list/export", session.Value)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// TestOAuth_SelfRegister_OwnerUnderOwnersAllowed: under owners_allowed an unknown
// verified email self-registers an owner account (ADR-0012 cut 2).
func TestOAuth_SelfRegister_OwnerUnderOwnersAllowed(t *testing.T) {
	o := newOAuthHarnessWithPolicy(t, "alice@example.com", true, storage.RegistrationOwnersAllowed)
	o.mock.next = mockIdentity{sub: "google-new-owner", email: "newowner@example.com", emailVerified: true}

	session := o.completeLogin(t, "")
	principal, err := o.authIssuerValidate(session.Value)
	require.NoError(t, err)
	assert.NotEqual(t, o.owner.ID, principal.UserID)

	ctx := context.Background()
	u, err := o.store.ForTenant(o.tenant).Users().ByEmail(ctx, "newowner@example.com")
	require.NoError(t, err)
	assert.Equal(t, storage.RoleOwner, u.Role)
	assert.Equal(t, storage.UserStatusActive, u.Status)
	assert.Empty(t, u.PasswordHash)
}

// TestOAuth_SelfRegister_RejectedWhenDisabled: with self-registration disabled, an
// unknown verified email stays a link-only rejection (no_owner) and provisions
// nothing — the pre-cut-2 behavior.
func TestOAuth_SelfRegister_RejectedWhenDisabled(t *testing.T) {
	o := newOAuthHarnessWithPolicy(t, "alice@example.com", true, storage.RegistrationDisabled)
	o.mock.next = mockIdentity{sub: "google-stranger", email: "stranger@example.com", emailVerified: true}

	callbackPath, stateCookie := o.runToCallback(t, "")
	rec := o.get(callbackPath, []*http.Cookie{stateCookie})
	o.assertLoginErrorRedirect(t, rec, "no_owner")

	// No account was created for the stranger email.
	_, err := o.store.ForTenant(o.tenant).Users().ByEmail(context.Background(), "stranger@example.com")
	assert.ErrorIs(t, err, storage.ErrNotFound)
}

// TestOAuth_SelfRegister_ExistingEmailLinksNotDuplicate: with self-registration
// enabled, a verified email that already has an account links that account rather than
// creating a second one — self-register only fires when there is no existing account.
func TestOAuth_SelfRegister_ExistingEmailLinksNotDuplicate(t *testing.T) {
	o := newOAuthHarnessWithPolicy(t, "alice@example.com", true, storage.RegistrationOwnersAllowed)
	o.mock.next = mockIdentity{sub: "google-alice", email: "alice@example.com", emailVerified: true}

	session := o.completeLogin(t, "")
	principal, err := o.authIssuerValidate(session.Value)
	require.NoError(t, err)
	assert.Equal(t, o.owner.ID, principal.UserID, "links the existing owner, not a new account")

	// Still exactly one account in the tenant (the seeded owner).
	_, total, err := o.store.ForTenant(o.tenant).Users().List(context.Background(), storage.Page{Limit: 100})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
}

// TestOAuth_SelfRegister_NoPasswordEstablishViaReset: an OAuth-created no-password
// account can set a first password through the forgot-password flow — cut 1b's widened
// resettable() covers the empty-hash account — and then log in with it.
func TestOAuth_SelfRegister_NoPasswordEstablishViaReset(t *testing.T) {
	o := newOAuthHarnessWithPolicy(t, "alice@example.com", true, storage.RegistrationGiversOnly)
	o.mock.next = mockIdentity{sub: "google-reset", email: "resetme@example.com", emailVerified: true}
	o.completeLogin(t, "") // provisions the no-password account

	host := o.tenantHost()

	// Forgot-password on the OAuth account's email sends an establish-password link.
	rec := o.postJSON(host, "/api/v1/auth/password-reset/request", map[string]any{"identifier": "resetme@example.com"})
	require.Equal(t, http.StatusAccepted, rec.Code)
	require.Eventually(t, func() bool { return len(o.email.Sent()) == 1 }, time.Second, 10*time.Millisecond)
	raw := resetTokenFromEmail(t, o.email.Sent()[0].Body)

	// Establish the first password (auto-login on confirm).
	rec = o.postJSON(host, "/api/v1/auth/password-reset/confirm", map[string]any{"token": raw, "new_password": "chosen-password"})
	require.Equal(t, http.StatusOK, rec.Code, "confirm body: %s", rec.Body.String())

	// The account can now log in with the password it just set.
	rec = o.postJSON(host, "/api/v1/auth/login", map[string]any{"username": "resetme@example.com", "password": "chosen-password"})
	assert.Equal(t, http.StatusOK, rec.Code, "the established password logs in")
}
