package sqlstore_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/storage"
)

func TestUserRoleAndBan(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ten := mkTenant(t, st, "alice")
	ts := st.ForTenant(ten)

	// A user created without a role defaults to owner (migration default parity).
	owner, err := ts.Users().Create(ctx, storage.User{Name: "Alice", Email: "a@example.com"})
	require.NoError(t, err)
	assert.Equal(t, storage.RoleOwner, owner.Role)
	assert.False(t, owner.Banned)

	giver, err := ts.Users().Create(ctx, storage.User{Name: "Gwen", Email: "g@example.com", Role: storage.RoleGiver})
	require.NoError(t, err)
	assert.Equal(t, storage.RoleGiver, giver.Role)

	// List returns both, and round-trips role/banned.
	users, total, err := ts.Users().List(ctx, storage.Page{Limit: 50})
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.Len(t, users, 2)

	// SetRole + SetBanned persist.
	require.NoError(t, ts.Users().SetRole(ctx, giver.ID, storage.RoleOwner))
	require.NoError(t, ts.Users().SetBanned(ctx, giver.ID, true))
	got, err := ts.Users().Get(ctx, giver.ID)
	require.NoError(t, err)
	assert.Equal(t, storage.RoleOwner, got.Role)
	assert.True(t, got.Banned)

	// Unknown user → ErrNotFound on the setters.
	assert.ErrorIs(t, ts.Users().SetRole(ctx, "nope", storage.RoleGiver), storage.ErrNotFound)
	assert.ErrorIs(t, ts.Users().SetBanned(ctx, "nope", true), storage.ErrNotFound)
}

func TestListTenants(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	mkTenant(t, st, "alice")
	mkTenant(t, st, "bob")

	tenants, total, err := st.ListTenants(ctx, storage.Page{Limit: 50})
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.Len(t, tenants, 2)
}
