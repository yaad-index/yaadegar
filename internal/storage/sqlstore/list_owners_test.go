package sqlstore_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/storage"
)

// TestListOwnership covers the join-table ownership model (ADR-0005 §7 / #30 Cut
// A2a): the creator is the sole owner, IsOwner/Owners resolve through the table,
// the derived List.OwnerID is populated on reads, the v1 single-owner rule is
// enforced, List() is owner-scoped, and delete clears the ownership rows.
func TestListOwnership(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ts := st.ForTenant(mkTenant(t, st, "alice"))

	o1, err := ts.Users().Create(ctx, storage.User{Name: "Owner"})
	require.NoError(t, err)
	o2, err := ts.Users().Create(ctx, storage.User{Name: "Other"})
	require.NoError(t, err)

	list, err := ts.Lists().Create(ctx, storage.List{Title: "L"}, o1.ID)
	require.NoError(t, err)

	// The creator is the sole owner.
	owns, err := ts.Lists().IsOwner(ctx, list.ID, o1.ID)
	require.NoError(t, err)
	assert.True(t, owns)
	owns, err = ts.Lists().IsOwner(ctx, list.ID, o2.ID)
	require.NoError(t, err)
	assert.False(t, owns)

	owners, err := ts.Lists().Owners(ctx, list.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{o1.ID}, owners)

	// The derived OwnerID is populated from the join table on reads.
	got, err := ts.Lists().Get(ctx, list.ID)
	require.NoError(t, err)
	assert.Equal(t, o1.ID, got.OwnerID)

	// v1 single-owner: a second owner is rejected; re-adding the sole owner is a no-op.
	assert.ErrorIs(t, ts.Lists().AddOwner(ctx, list.ID, o2.ID), storage.ErrConflict)
	require.NoError(t, ts.Lists().AddOwner(ctx, list.ID, o1.ID))

	// List() is scoped to lists the user owns.
	ls, total, err := ts.Lists().List(ctx, o1.ID, storage.Page{Limit: 100})
	require.NoError(t, err)
	assert.Len(t, ls, 1)
	assert.Equal(t, 1, total)
	ls, total, err = ts.Lists().List(ctx, o2.ID, storage.Page{Limit: 100})
	require.NoError(t, err)
	assert.Empty(t, ls)
	assert.Zero(t, total)

	// Delete clears the ownership rows (app-level cleanup backing the FK cascade).
	require.NoError(t, ts.Lists().Delete(ctx, list.ID))
	owners, err = ts.Lists().Owners(ctx, list.ID)
	require.NoError(t, err)
	assert.Empty(t, owners)
}
