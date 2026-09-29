package sqlstore_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/storage"
)

func mkAccessToken(userID, name, hash string, expires *time.Time) storage.AccessToken {
	return storage.AccessToken{UserID: userID, Name: name, TokenHash: hash, Last4: hash[len(hash)-4:], ExpiresAt: expires}
}

// TestAccessTokensRoundTrip covers ADR-0016's stored shape: the hash and last four
// characters are stored, never the raw value; nil expiry and nil last-used survive.
func TestAccessTokensRoundTrip(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ts := st.ForTenant(mkTenant(t, st, "alice"))
	user, err := ts.Users().Create(ctx, storage.User{Name: "Alice"})
	require.NoError(t, err)
	now := time.Date(2027, 1, 10, 9, 0, 0, 0, time.UTC)

	created, err := ts.AccessTokens().Create(ctx, mkAccessToken(user.ID, "backup script", "hash-0001", nil), 20, now)
	require.NoError(t, err)
	assert.NotEmpty(t, created.ID)

	got, err := ts.AccessTokens().ByHash(ctx, "hash-0001")
	require.NoError(t, err)
	assert.Equal(t, "backup script", got.Name)
	assert.Equal(t, "0001", got.Last4)
	assert.Nil(t, got.ExpiresAt, "no expiry round-trips as nil")
	assert.Nil(t, got.LastUsedAt)
	assert.Nil(t, got.RevokedAt)
	assert.True(t, got.ActiveAt(now))

	_, err = ts.AccessTokens().ByHash(ctx, "unknown")
	assert.ErrorIs(t, err, storage.ErrNotFound)
}

// TestAccessTokensLimit covers ADR-0016 §6: the 21st active token is refused, and
// revoked or expired tokens do not count against the limit.
func TestAccessTokensLimit(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ts := st.ForTenant(mkTenant(t, st, "alice"))
	user, err := ts.Users().Create(ctx, storage.User{Name: "Alice"})
	require.NoError(t, err)
	now := time.Date(2027, 1, 10, 9, 0, 0, 0, time.UTC)

	var first storage.AccessToken
	for i := 0; i < 20; i++ {
		tok, err := ts.AccessTokens().Create(ctx, mkAccessToken(user.ID, "t", fmt.Sprintf("hash-%04d", i), nil), 20, now)
		require.NoError(t, err)
		if i == 0 {
			first = tok
		}
	}
	_, err = ts.AccessTokens().Create(ctx, mkAccessToken(user.ID, "t", "hash-0020", nil), 20, now)
	assert.ErrorIs(t, err, storage.ErrTooManyTokens, "the 21st active token is refused")

	revoked, err := ts.AccessTokens().Revoke(ctx, user.ID, first.ID, now)
	require.NoError(t, err)
	require.True(t, revoked)
	_, err = ts.AccessTokens().Create(ctx, mkAccessToken(user.ID, "t", "hash-0021", nil), 20, now)
	require.NoError(t, err, "a revoked token frees its slot")

	// An expired token does not count either: fill up with one that expires, then
	// create again after its expiry.
	soon := now.Add(time.Hour)
	require.NoError(t, revokeOne(ctx, ts, user.ID, now))
	_, err = ts.AccessTokens().Create(ctx, mkAccessToken(user.ID, "t", "hash-0022", &soon), 20, now)
	require.NoError(t, err)
	_, err = ts.AccessTokens().Create(ctx, mkAccessToken(user.ID, "t", "hash-0023", nil), 20, now)
	assert.ErrorIs(t, err, storage.ErrTooManyTokens, "an unexpired token still counts")
	_, err = ts.AccessTokens().Create(ctx, mkAccessToken(user.ID, "t", "hash-0024", nil), 20, soon.Add(time.Second))
	require.NoError(t, err, "an expired token frees its slot")
}

// revokeOne revokes the newest active token of the user.
func revokeOne(ctx context.Context, ts storage.TenantStore, userID string, now time.Time) error {
	list, err := ts.AccessTokens().ListByUser(ctx, userID)
	if err != nil {
		return err
	}
	for _, tok := range list {
		if tok.ActiveAt(now) {
			_, err := ts.AccessTokens().Revoke(ctx, userID, tok.ID, now)
			return err
		}
	}
	return nil
}

// TestAccessTokensConcurrentCreateRespectsLimit: concurrent creations for the last
// slot all return a correct result, exactly one success and the rest refused.
// SQLite runs one connection, so this cannot exercise the row lock itself; the
// race is real only on Postgres, and TestPostgres_AccessTokenLimitSingleWinner
// covers it there.
func TestAccessTokensConcurrentCreateRespectsLimit(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ts := st.ForTenant(mkTenant(t, st, "alice"))
	user, err := ts.Users().Create(ctx, storage.User{Name: "Alice"})
	require.NoError(t, err)
	now := time.Date(2027, 1, 10, 9, 0, 0, 0, time.UTC)
	for i := 0; i < 19; i++ {
		_, err := ts.AccessTokens().Create(ctx, mkAccessToken(user.ID, "t", fmt.Sprintf("hash-%04d", i), nil), 20, now)
		require.NoError(t, err)
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	ok, refused := 0, 0
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := ts.AccessTokens().Create(ctx, mkAccessToken(user.ID, "t", fmt.Sprintf("race-%04d", i), nil), 20, now)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				ok++
			case assert.ErrorIs(t, err, storage.ErrTooManyTokens):
				refused++
			}
		}(i)
	}
	wg.Wait()
	assert.Equal(t, 1, ok, "exactly one creation takes the last slot")
	assert.Equal(t, 7, refused)
}

// TestAccessTokensRevoke: revocation is scoped to the owning user, idempotent, and
// revoke-all touches only that user's tokens.
func TestAccessTokensRevoke(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ts := st.ForTenant(mkTenant(t, st, "alice"))
	alice, err := ts.Users().Create(ctx, storage.User{Name: "Alice"})
	require.NoError(t, err)
	bob, err := ts.Users().Create(ctx, storage.User{Name: "Bob"})
	require.NoError(t, err)
	now := time.Date(2027, 1, 10, 9, 0, 0, 0, time.UTC)

	a1, err := ts.AccessTokens().Create(ctx, mkAccessToken(alice.ID, "a1", "hash-a001", nil), 20, now)
	require.NoError(t, err)
	_, err = ts.AccessTokens().Create(ctx, mkAccessToken(alice.ID, "a2", "hash-a002", nil), 20, now)
	require.NoError(t, err)
	b1, err := ts.AccessTokens().Create(ctx, mkAccessToken(bob.ID, "b1", "hash-b001", nil), 20, now)
	require.NoError(t, err)

	_, err = ts.AccessTokens().Revoke(ctx, bob.ID, a1.ID, now)
	assert.ErrorIs(t, err, storage.ErrNotFound, "one account cannot revoke another's token")

	revoked, err := ts.AccessTokens().Revoke(ctx, alice.ID, a1.ID, now)
	require.NoError(t, err)
	assert.True(t, revoked)
	revoked, err = ts.AccessTokens().Revoke(ctx, alice.ID, a1.ID, now)
	require.NoError(t, err)
	assert.False(t, revoked, "revoking twice reports false")

	n, err := ts.AccessTokens().RevokeAllForUser(ctx, alice.ID, now)
	require.NoError(t, err)
	assert.Equal(t, int64(1), n, "only the one still-active token of alice")

	got, err := ts.AccessTokens().ByHash(ctx, "hash-b001")
	require.NoError(t, err)
	assert.Nil(t, got.RevokedAt, "bob's token is untouched")
	assert.Equal(t, b1.ID, got.ID)

	list, err := ts.AccessTokens().ListByUser(ctx, alice.ID)
	require.NoError(t, err)
	require.Len(t, list, 2, "revoked tokens are still listed")
	for _, tok := range list {
		assert.NotNil(t, tok.RevokedAt)
		assert.False(t, tok.ActiveAt(now))
	}
}

// TestAccessTokensTouchLastUsed: a use is written at most once per interval.
func TestAccessTokensTouchLastUsed(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ts := st.ForTenant(mkTenant(t, st, "alice"))
	user, err := ts.Users().Create(ctx, storage.User{Name: "Alice"})
	require.NoError(t, err)
	now := time.Date(2027, 1, 10, 9, 0, 0, 0, time.UTC)
	tok, err := ts.AccessTokens().Create(ctx, mkAccessToken(user.ID, "t", "hash-0001", nil), 20, now)
	require.NoError(t, err)

	wrote, err := ts.AccessTokens().TouchLastUsed(ctx, tok.ID, now, 5*time.Minute)
	require.NoError(t, err)
	assert.True(t, wrote, "first use is recorded")

	wrote, err = ts.AccessTokens().TouchLastUsed(ctx, tok.ID, now.Add(4*time.Minute), 5*time.Minute)
	require.NoError(t, err)
	assert.False(t, wrote, "within the interval nothing is written")

	wrote, err = ts.AccessTokens().TouchLastUsed(ctx, tok.ID, now.Add(5*time.Minute), 5*time.Minute)
	require.NoError(t, err)
	assert.True(t, wrote, "at the interval the use is recorded again")

	got, err := ts.AccessTokens().ByHash(ctx, "hash-0001")
	require.NoError(t, err)
	require.NotNil(t, got.LastUsedAt)
	assert.True(t, got.LastUsedAt.Equal(now.Add(5*time.Minute)))

	_, err = ts.AccessTokens().TouchLastUsed(ctx, "missing", now, 5*time.Minute)
	assert.ErrorIs(t, err, storage.ErrNotFound)
}

// TestAccessTokensTenantIsolation: a token is invisible from another tenant.
func TestAccessTokensTenantIsolation(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	alice := st.ForTenant(mkTenant(t, st, "alice"))
	bobT := st.ForTenant(mkTenant(t, st, "bob"))
	user, err := alice.Users().Create(ctx, storage.User{Name: "Alice"})
	require.NoError(t, err)
	now := time.Date(2027, 1, 10, 9, 0, 0, 0, time.UTC)
	_, err = alice.AccessTokens().Create(ctx, mkAccessToken(user.ID, "t", "hash-0001", nil), 20, now)
	require.NoError(t, err)

	_, err = bobT.AccessTokens().ByHash(ctx, "hash-0001")
	assert.ErrorIs(t, err, storage.ErrNotFound)
	list, err := bobT.AccessTokens().ListByUser(ctx, user.ID)
	require.NoError(t, err)
	assert.Empty(t, list)
}
