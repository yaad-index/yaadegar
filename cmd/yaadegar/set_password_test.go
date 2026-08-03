package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/auth"
	"github.com/yaad-index/yaadegar/internal/storage"
)

// tempStorageFlags points the seed commands at a fresh throwaway sqlite file.
func tempStorageFlags(t *testing.T) storageFlags {
	t.Helper()
	return storageFlags{
		StorageDriver: "sqlite",
		StorageDSN:    "file:" + filepath.Join(t.TempDir(), "test.db"),
	}
}

// seedOwner creates a tenant + a credentialed owner with the given password, then
// closes the setup store so the command under test opens its own connection.
func seedOwner(t *testing.T, sf storageFlags, subdomain, username, password string) {
	t.Helper()
	ctx := context.Background()
	store, err := sf.open(ctx)
	require.NoError(t, err)
	tenant, err := store.CreateTenant(ctx, storage.Tenant{Subdomain: subdomain})
	require.NoError(t, err)
	hash, err := auth.HashPassword(password)
	require.NoError(t, err)
	uname := username
	_, err = store.ForTenant(tenant).Users().Create(ctx, storage.User{
		Name:         username,
		Username:     &uname,
		PasswordHash: hash,
	})
	require.NoError(t, err)
	require.NoError(t, store.Close())
}

func TestSetPasswordUpdatesCredential(t *testing.T) {
	sf := tempStorageFlags(t)
	seedOwner(t, sf, "acme", "owner", "old-pw")

	// set-password reads the new password from the environment. It must satisfy the
	// shared password policy (ADR-0011): at least auth.MinPasswordLen characters.
	t.Setenv("YAADEGAR_PASSWORD", "new-password")
	require.NoError(t, (&SetPasswordCmd{storageFlags: sf, Tenant: "acme", Username: "owner"}).Run())

	// The stored credential now verifies against the new password, and the old one
	// no longer works — a login with the new password succeeds, the old fails.
	ctx := context.Background()
	store, err := sf.open(ctx)
	require.NoError(t, err)
	defer func() { _ = store.Close() }()
	tenant, err := store.TenantBySubdomain(ctx, "acme")
	require.NoError(t, err)
	user, err := store.ForTenant(tenant).Users().ByUsername(ctx, "owner")
	require.NoError(t, err)

	okNew, err := auth.VerifyPassword("new-password", user.PasswordHash)
	require.NoError(t, err)
	assert.True(t, okNew, "new password verifies after set-password")
	okOld, err := auth.VerifyPassword("old-pw", user.PasswordHash)
	require.NoError(t, err)
	assert.False(t, okOld, "old password no longer verifies")

	// set-password bumps credential_version (ADR-0011), so prior sessions are revoked:
	// the owner started at 1, and one reset moves it to 2.
	assert.Equal(t, 2, user.CredentialVersion)
}

// TestSetPasswordRejectsShortPassword: the shared policy is enforced on the CLI path
// (ADR-0011 §4) — a too-short new password is refused and the credential is unchanged.
func TestSetPasswordRejectsShortPassword(t *testing.T) {
	sf := tempStorageFlags(t)
	seedOwner(t, sf, "acme", "owner", "old-pw")

	t.Setenv("YAADEGAR_PASSWORD", "short") // below auth.MinPasswordLen
	err := (&SetPasswordCmd{storageFlags: sf, Tenant: "acme", Username: "owner"}).Run()
	require.Error(t, err)
	assert.ErrorIs(t, err, auth.ErrPasswordTooShort)
}

func TestSetPasswordUnknownTenantOrUser(t *testing.T) {
	sf := tempStorageFlags(t)
	seedOwner(t, sf, "acme", "owner", "pw")
	t.Setenv("YAADEGAR_PASSWORD", "new-pw")

	err := (&SetPasswordCmd{storageFlags: sf, Tenant: "ghost", Username: "owner"}).Run()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no tenant")

	err = (&SetPasswordCmd{storageFlags: sf, Tenant: "acme", Username: "ghost"}).Run()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no user")
}
