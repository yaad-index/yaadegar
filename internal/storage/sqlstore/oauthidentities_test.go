package sqlstore_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/storage"
)

func TestOAuthIdentity_CreateAndLookup(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ts, owner, _ := seedList(t, st)

	created, err := ts.OAuthIdentities().Create(ctx, storage.OAuthIdentity{
		UserID:   owner.ID,
		Provider: storage.OAuthProviderGoogle,
		Subject:  "sub-123",
		Email:    "alice@example.com",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, created.ID)
	assert.False(t, created.CreatedAt.IsZero())

	bySub, err := ts.OAuthIdentities().ByProviderSubject(ctx, storage.OAuthProviderGoogle, "sub-123")
	require.NoError(t, err)
	assert.Equal(t, owner.ID, bySub.UserID)
	assert.Equal(t, "alice@example.com", bySub.Email)

	byUser, err := ts.OAuthIdentities().ByUserProvider(ctx, owner.ID, storage.OAuthProviderGoogle)
	require.NoError(t, err)
	assert.Equal(t, "sub-123", byUser.Subject)
}

func TestOAuthIdentity_NotFound(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ts, owner, _ := seedList(t, st)

	_, err := ts.OAuthIdentities().ByProviderSubject(ctx, storage.OAuthProviderGoogle, "nope")
	assert.ErrorIs(t, err, storage.ErrNotFound)
	_, err = ts.OAuthIdentities().ByUserProvider(ctx, owner.ID, storage.OAuthProviderGoogle)
	assert.ErrorIs(t, err, storage.ErrNotFound)
}

func TestOAuthIdentity_UniqueSubjectPerTenant(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ts, owner, _ := seedList(t, st)

	_, err := ts.OAuthIdentities().Create(ctx, storage.OAuthIdentity{
		UserID: owner.ID, Provider: storage.OAuthProviderGoogle, Subject: "dup", Email: "a@example.com",
	})
	require.NoError(t, err)
	// Same (provider, subject) within the tenant conflicts.
	_, err = ts.OAuthIdentities().Create(ctx, storage.OAuthIdentity{
		UserID: owner.ID, Provider: storage.OAuthProviderGoogle, Subject: "dup", Email: "a@example.com",
	})
	assert.ErrorIs(t, err, storage.ErrConflict)
}

// The uniqueness is tenant-scoped, NOT global: the same Google subject may link in
// two different tenants on one instance (ADR-0008 §5).
func TestOAuthIdentity_SameSubjectDifferentTenants(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	tenA := mkTenant(t, st, "alice")
	tsA := st.ForTenant(tenA)
	ownerA, err := tsA.Users().Create(ctx, storage.User{Name: "Alice"})
	require.NoError(t, err)

	tenB := mkTenant(t, st, "bob")
	tsB := st.ForTenant(tenB)
	ownerB, err := tsB.Users().Create(ctx, storage.User{Name: "Bob"})
	require.NoError(t, err)

	_, err = tsA.OAuthIdentities().Create(ctx, storage.OAuthIdentity{
		UserID: ownerA.ID, Provider: storage.OAuthProviderGoogle, Subject: "shared", Email: "x@example.com",
	})
	require.NoError(t, err)
	// Same subject links cleanly in the other tenant.
	_, err = tsB.OAuthIdentities().Create(ctx, storage.OAuthIdentity{
		UserID: ownerB.ID, Provider: storage.OAuthProviderGoogle, Subject: "shared", Email: "x@example.com",
	})
	require.NoError(t, err)

	// And a tenant only sees its own link.
	_, err = tsA.OAuthIdentities().ByProviderSubject(ctx, storage.OAuthProviderGoogle, "shared")
	require.NoError(t, err)
}

func TestUserByEmailAndTenantOAuthToggle(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ten := mkTenant(t, st, "alice")
	ts := st.ForTenant(ten)

	_, err := ts.Users().Create(ctx, storage.User{Name: "Alice", Email: "alice@example.com"})
	require.NoError(t, err)

	got, err := ts.Users().ByEmail(ctx, "alice@example.com")
	require.NoError(t, err)
	assert.Equal(t, "Alice", got.Name)

	_, err = ts.Users().ByEmail(ctx, "missing@example.com")
	assert.ErrorIs(t, err, storage.ErrNotFound)
	// An empty email never matches (would otherwise link email-less accounts).
	_, err = ts.Users().ByEmail(ctx, "")
	assert.ErrorIs(t, err, storage.ErrNotFound)

	// Toggle defaults off, flips on, reflected on reload.
	assert.False(t, ten.OAuthGoogleEnabled)
	require.NoError(t, st.SetTenantOAuthGoogle(ctx, ten.ID, true))
	reloaded, err := st.TenantByID(ctx, ten.ID)
	require.NoError(t, err)
	assert.True(t, reloaded.OAuthGoogleEnabled)

	// Unknown tenant is ErrNotFound.
	assert.ErrorIs(t, st.SetTenantOAuthGoogle(ctx, "no-such-tenant", true), storage.ErrNotFound)
}
