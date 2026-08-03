package sqlstore_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/storage"
)

// TestCredentialVersion covers the ADR-0011 foundation: a fresh account starts at
// version 1, and SetPasswordHash bumps the version in the same write so a password
// mutation revokes prior sessions.
func TestCredentialVersion(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ten := mkTenant(t, st, "alice")
	ts := st.ForTenant(ten)

	// A new account starts at version 1 (the migration default; Create seeds it).
	user, err := ts.Users().Create(ctx, storage.User{Name: "Alice"})
	require.NoError(t, err)
	assert.Equal(t, 1, user.CredentialVersion)

	got, err := ts.Users().Get(ctx, user.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, got.CredentialVersion)

	// SetPasswordHash bumps the version in the same write.
	require.NoError(t, ts.Users().SetPasswordHash(ctx, user.ID, "new-hash"))
	got, err = ts.Users().Get(ctx, user.ID)
	require.NoError(t, err)
	assert.Equal(t, 2, got.CredentialVersion)
	assert.Equal(t, "new-hash", got.PasswordHash)

	// A second reset bumps again — monotonic.
	require.NoError(t, ts.Users().SetPasswordHash(ctx, user.ID, "newer-hash"))
	got, err = ts.Users().Get(ctx, user.ID)
	require.NoError(t, err)
	assert.Equal(t, 3, got.CredentialVersion)

	// Unknown user → ErrNotFound (no silent no-op).
	assert.ErrorIs(t, ts.Users().SetPasswordHash(ctx, "nope", "h"), storage.ErrNotFound)
}
