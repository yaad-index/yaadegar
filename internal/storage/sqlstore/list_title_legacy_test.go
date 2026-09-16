package sqlstore_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/storage"
	"github.com/yaad-index/yaadegar/internal/storage/sqlstore"
)

// A row written before the #406 rule existed can hold a padded title, and nothing
// reachable through the repo can produce one any more — so this test puts one there
// with raw SQL, which is the only way to stand up the state it is about.
//
// What it pins is a consequence of trimming on the write path rather than only at
// the title's own edit: an update that does not mention the title still carries the
// stored value through storedTitle, so a legacy padded title normalises on the next
// unrelated write. That is the intended behaviour and it is self-healing, but an
// unrelated write quietly changing a field is worth a test naming it rather than a
// reader discovering it.
func TestListTitle_LegacyPaddedRowNormalisesOnUnrelatedUpdate(t *testing.T) {
	ctx := context.Background()
	dsn := "file:" + filepath.Join(t.TempDir(), "legacy.db")
	st, err := sqlstore.Open(ctx, storage.Config{Driver: storage.DriverSQLite, DSN: dsn})
	require.NoError(t, err)
	require.NoError(t, st.Migrate(ctx))
	t.Cleanup(func() { _ = st.Close() })

	ten := mkTenant(t, st, "legacy")
	ts := st.ForTenant(ten)
	owner, err := ts.Users().Create(ctx, storage.User{Name: "Owner"})
	require.NoError(t, err)
	list, err := ts.Lists().Create(ctx, storage.List{Title: "Padded later"}, owner.ID)
	require.NoError(t, err)

	// Reach past the repo to create the state the rule now prevents.
	raw, err := sql.Open("sqlite", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = raw.Close() })
	_, err = raw.ExecContext(ctx, `UPDATE lists SET title = ? WHERE id = ?`, "  Legacy  ", list.ID)
	require.NoError(t, err)

	// Confirm the padded row really is there, or the rest of the test proves nothing.
	stale, err := ts.Lists().Get(ctx, list.ID)
	require.NoError(t, err)
	require.Equal(t, "  Legacy  ", stale.Title, "the raw update did not take")

	// An update that says nothing about the title: it rides along from the loaded row.
	stale.Visibility = storage.VisibilityPublic
	updated, err := ts.Lists().Update(ctx, stale)
	require.NoError(t, err)
	assert.Equal(t, "Legacy", updated.Title)

	got, err := ts.Lists().Get(ctx, list.ID)
	require.NoError(t, err)
	assert.Equal(t, "Legacy", got.Title)
	assert.Equal(t, storage.VisibilityPublic, got.Visibility)
}
