package sqlstore_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/storage"
)

// TestEmailVerificationTokens covers the ADR-0012 cut-1a store: round-trip, and the
// single-use claim (MarkUsed sets used_at only while NULL, so exactly one claim wins).
func TestEmailVerificationTokens(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ten := mkTenant(t, st, "alice")
	ts := st.ForTenant(ten)
	user, err := ts.Users().Create(ctx, storage.User{Name: "Alice"})
	require.NoError(t, err)

	exp := time.Date(2027, 6, 15, 13, 0, 0, 0, time.UTC)
	created, err := ts.EmailVerificationTokens().Create(ctx, storage.EmailVerificationToken{
		UserID: user.ID, TokenHash: "hash-abc", ExpiresAt: exp,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, created.ID)

	// ByHash round-trips the row; used_at is nil.
	got, err := ts.EmailVerificationTokens().ByHash(ctx, "hash-abc")
	require.NoError(t, err)
	assert.Equal(t, user.ID, got.UserID)
	assert.True(t, got.ExpiresAt.Equal(exp))
	assert.Nil(t, got.UsedAt)

	// Unknown hash → ErrNotFound.
	_, err = ts.EmailVerificationTokens().ByHash(ctx, "nope")
	assert.ErrorIs(t, err, storage.ErrNotFound)

	// First claim wins; the second finds nothing to claim (single-use).
	now := time.Date(2027, 6, 15, 12, 30, 0, 0, time.UTC)
	claimed, err := ts.EmailVerificationTokens().MarkUsed(ctx, created.ID, now)
	require.NoError(t, err)
	assert.True(t, claimed)

	claimed, err = ts.EmailVerificationTokens().MarkUsed(ctx, created.ID, now)
	require.NoError(t, err)
	assert.False(t, claimed, "a used token cannot be claimed again")

	// The row now reports used_at.
	got, err = ts.EmailVerificationTokens().ByHash(ctx, "hash-abc")
	require.NoError(t, err)
	require.NotNil(t, got.UsedAt)

	// MarkUsed on an absent id → ErrNotFound.
	_, err = ts.EmailVerificationTokens().MarkUsed(ctx, "no-such-id", now)
	assert.ErrorIs(t, err, storage.ErrNotFound)
}

// TestEmailVerificationLatestAndDelete covers the resend-verification helpers (#162):
// LatestByUser returns the newest token (for the anti-flood CreatedAt), and
// DeleteByUser clears the user's tokens so a re-mint fully replaces the prior one.
func TestEmailVerificationLatestAndDelete(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ten := mkTenant(t, st, "alice")
	ts := st.ForTenant(ten)
	user, err := ts.Users().Create(ctx, storage.User{Name: "Alice"})
	require.NoError(t, err)
	repo := ts.EmailVerificationTokens()

	// No tokens yet → ErrNotFound.
	_, err = repo.LatestByUser(ctx, user.ID)
	assert.ErrorIs(t, err, storage.ErrNotFound)

	exp := time.Date(2027, 6, 15, 13, 0, 0, 0, time.UTC)
	older := time.Date(2027, 6, 15, 12, 0, 0, 0, time.UTC)
	newer := time.Date(2027, 6, 15, 12, 30, 0, 0, time.UTC)
	_, err = repo.Create(ctx, storage.EmailVerificationToken{
		UserID: user.ID, TokenHash: "hash-old", ExpiresAt: exp, CreatedAt: older,
	})
	require.NoError(t, err)
	_, err = repo.Create(ctx, storage.EmailVerificationToken{
		UserID: user.ID, TokenHash: "hash-new", ExpiresAt: exp, CreatedAt: newer,
	})
	require.NoError(t, err)

	// LatestByUser returns the most recently created token.
	latest, err := repo.LatestByUser(ctx, user.ID)
	require.NoError(t, err)
	assert.Equal(t, "hash-new", latest.TokenHash)
	assert.True(t, latest.CreatedAt.Equal(newer))

	// DeleteByUser clears every token; both lookups then miss.
	require.NoError(t, repo.DeleteByUser(ctx, user.ID))
	_, err = repo.ByHash(ctx, "hash-old")
	assert.ErrorIs(t, err, storage.ErrNotFound)
	_, err = repo.LatestByUser(ctx, user.ID)
	assert.ErrorIs(t, err, storage.ErrNotFound)

	// DeleteByUser on a user with no tokens is a no-op, not an error.
	require.NoError(t, repo.DeleteByUser(ctx, user.ID))
}

// TestUserStatusDefaultAndSetStatus: Create defaults an account to active, and
// SetStatus flips it (the pending → active activation, ADR-0012).
func TestUserStatusDefaultAndSetStatus(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ten := mkTenant(t, st, "alice")
	ts := st.ForTenant(ten)

	user, err := ts.Users().Create(ctx, storage.User{Name: "Alice"})
	require.NoError(t, err)
	assert.Equal(t, storage.UserStatusActive, user.Status, "Create defaults status to active")

	got, err := ts.Users().Get(ctx, user.ID)
	require.NoError(t, err)
	assert.Equal(t, storage.UserStatusActive, got.Status)

	// A pending account round-trips as pending, then SetStatus activates it.
	pending, err := ts.Users().Create(ctx, storage.User{Name: "Bob", Status: storage.UserStatusPending})
	require.NoError(t, err)
	assert.Equal(t, storage.UserStatusPending, pending.Status)

	require.NoError(t, ts.Users().SetStatus(ctx, pending.ID, storage.UserStatusActive))
	got, err = ts.Users().Get(ctx, pending.ID)
	require.NoError(t, err)
	assert.Equal(t, storage.UserStatusActive, got.Status)

	// SetStatus on an absent id → ErrNotFound.
	err = ts.Users().SetStatus(ctx, "no-such-id", storage.UserStatusActive)
	assert.ErrorIs(t, err, storage.ErrNotFound)
}
