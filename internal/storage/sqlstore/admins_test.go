package sqlstore_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/storage"
)

// TestEnsureAdminIdempotent covers the instance-level superadmin store (ADR-0005
// §6 / #30 Cut A2b): EnsureAdmin creates on first call and refreshes the password
// hash (same id) on a repeat, and both lookups resolve it.
func TestEnsureAdminIdempotent(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	a1, err := st.EnsureAdmin(ctx, "root", "hash-v1")
	require.NoError(t, err)
	assert.NotEmpty(t, a1.ID)
	assert.Equal(t, "root", a1.Username)
	assert.Equal(t, "hash-v1", a1.PasswordHash)

	// Idempotent: same username keeps the id, refreshes the hash (rotation).
	a2, err := st.EnsureAdmin(ctx, "root", "hash-v2")
	require.NoError(t, err)
	assert.Equal(t, a1.ID, a2.ID, "id is stable across re-ensure")
	assert.Equal(t, "hash-v2", a2.PasswordHash)

	byName, err := st.AdminByUsername(ctx, "root")
	require.NoError(t, err)
	assert.Equal(t, a1.ID, byName.ID)
	assert.Equal(t, "hash-v2", byName.PasswordHash)

	byID, err := st.AdminByID(ctx, a1.ID)
	require.NoError(t, err)
	assert.Equal(t, "root", byID.Username)

	_, err = st.AdminByUsername(ctx, "nobody")
	assert.ErrorIs(t, err, storage.ErrNotFound)
}
