package sqlstore_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/storage"
)

// TestPasswordResetTokens covers the ADR-0011 cut-3 store: round-trip, and the
// single-use claim (MarkUsed sets used_at only while NULL, so exactly one claim wins).
func TestPasswordResetTokens(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ten := mkTenant(t, st, "alice")
	ts := st.ForTenant(ten)
	user, err := ts.Users().Create(ctx, storage.User{Name: "Alice"})
	require.NoError(t, err)

	exp := time.Date(2027, 6, 15, 13, 0, 0, 0, time.UTC)
	created, err := ts.PasswordResetTokens().Create(ctx, storage.PasswordResetToken{
		UserID: user.ID, TokenHash: "hash-abc", ExpiresAt: exp,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, created.ID)

	// ByHash round-trips the row; used_at is nil.
	got, err := ts.PasswordResetTokens().ByHash(ctx, "hash-abc")
	require.NoError(t, err)
	assert.Equal(t, user.ID, got.UserID)
	assert.True(t, got.ExpiresAt.Equal(exp))
	assert.Nil(t, got.UsedAt)

	// Unknown hash → ErrNotFound.
	_, err = ts.PasswordResetTokens().ByHash(ctx, "nope")
	assert.ErrorIs(t, err, storage.ErrNotFound)

	// First claim wins; the second finds nothing to claim (single-use).
	now := time.Date(2027, 6, 15, 12, 30, 0, 0, time.UTC)
	claimed, err := ts.PasswordResetTokens().MarkUsed(ctx, created.ID, now)
	require.NoError(t, err)
	assert.True(t, claimed)

	claimed, err = ts.PasswordResetTokens().MarkUsed(ctx, created.ID, now)
	require.NoError(t, err)
	assert.False(t, claimed, "a used token cannot be claimed again")

	// The row now reports used_at.
	got, err = ts.PasswordResetTokens().ByHash(ctx, "hash-abc")
	require.NoError(t, err)
	require.NotNil(t, got.UsedAt)

	// MarkUsed on an absent id → ErrNotFound.
	_, err = ts.PasswordResetTokens().MarkUsed(ctx, "no-such-id", now)
	assert.ErrorIs(t, err, storage.ErrNotFound)
}

// TestConfirmResetAtomic covers #166: ConfirmReset applies the confirm mutation set
// (set password + activate + claim token) atomically. The happy path establishes the
// password and activates a pending account; when the token was already claimed, the
// whole set rolls back so the account never lands in a partial state.
func TestConfirmResetAtomic(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ten := mkTenant(t, st, "alice")
	ts := st.ForTenant(ten)
	exp := time.Date(2027, 6, 15, 13, 0, 0, 0, time.UTC)
	now := time.Date(2027, 6, 15, 12, 30, 0, 0, time.UTC)

	t.Run("happy path establishes password and activates", func(t *testing.T) {
		// A pending, no-password account (admin-invited / OAuth-established shape).
		user, err := ts.Users().Create(ctx, storage.User{Name: "Bob", Status: storage.UserStatusPending})
		require.NoError(t, err)
		require.Equal(t, 1, user.CredentialVersion)
		tok, err := ts.PasswordResetTokens().Create(ctx, storage.PasswordResetToken{
			UserID: user.ID, TokenHash: "hash-happy", ExpiresAt: exp,
		})
		require.NoError(t, err)

		claimed, err := ts.PasswordResetTokens().ConfirmReset(ctx, tok.ID, user.ID, "estab-hash", now)
		require.NoError(t, err)
		assert.True(t, claimed)

		got, err := ts.Users().Get(ctx, user.ID)
		require.NoError(t, err)
		assert.Equal(t, "estab-hash", got.PasswordHash, "password set")
		assert.Equal(t, 2, got.CredentialVersion, "credential_version bumped → sessions drop")
		assert.Equal(t, storage.UserStatusActive, got.Status, "pending account activated")

		prt, err := ts.PasswordResetTokens().ByHash(ctx, "hash-happy")
		require.NoError(t, err)
		assert.NotNil(t, prt.UsedAt, "token consumed")
	})

	t.Run("already-claimed token rolls the whole set back", func(t *testing.T) {
		user, err := ts.Users().Create(ctx, storage.User{Name: "Carol", Status: storage.UserStatusPending})
		require.NoError(t, err)
		tok, err := ts.PasswordResetTokens().Create(ctx, storage.PasswordResetToken{
			UserID: user.ID, TokenHash: "hash-claimed", ExpiresAt: exp,
		})
		require.NoError(t, err)
		// A concurrent confirm already consumed the token.
		claimed, err := ts.PasswordResetTokens().MarkUsed(ctx, tok.ID, now)
		require.NoError(t, err)
		require.True(t, claimed)

		// The commit gate finds the token used → no session-relevant writes persist.
		claimed, err = ts.PasswordResetTokens().ConfirmReset(ctx, tok.ID, user.ID, "estab-hash", now)
		require.NoError(t, err)
		assert.False(t, claimed, "a used token cannot be re-claimed")

		got, err := ts.Users().Get(ctx, user.ID)
		require.NoError(t, err)
		assert.Empty(t, got.PasswordHash, "password NOT set (rolled back)")
		assert.Equal(t, 1, got.CredentialVersion, "credential_version unchanged (rolled back)")
		assert.Equal(t, storage.UserStatusPending, got.Status, "status unchanged (rolled back)")
	})

	t.Run("unknown user → ErrNotFound, nothing written", func(t *testing.T) {
		tok, err := ts.PasswordResetTokens().Create(ctx, storage.PasswordResetToken{
			UserID: "ghost", TokenHash: "hash-ghost", ExpiresAt: exp,
		})
		require.NoError(t, err)
		claimed, err := ts.PasswordResetTokens().ConfirmReset(ctx, tok.ID, "ghost", "estab-hash", now)
		assert.ErrorIs(t, err, storage.ErrNotFound)
		assert.False(t, claimed)
		// The token is untouched — a missing user must not consume it.
		prt, err := ts.PasswordResetTokens().ByHash(ctx, "hash-ghost")
		require.NoError(t, err)
		assert.Nil(t, prt.UsedAt)
	})
}
