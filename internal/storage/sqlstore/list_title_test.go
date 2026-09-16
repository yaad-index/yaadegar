package sqlstore_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/storage"
)

// #406: the title rule sits on the storage write path because those two statements
// are the only way a title reaches the lists table. These cases pin it at that
// floor — below the API, below the web forms — so a future caller cannot reach the
// table around it.
func TestListTitle_StorageFloor(t *testing.T) {
	ctx := context.Background()
	ts, owner, list := seedList(t, newTestStore(t))

	t.Run("create refuses a title that is empty or only whitespace", func(t *testing.T) {
		// The last case is a non-breaking space: whitespace to unicode.IsSpace, and
		// not to SQL's btrim, so it is the one a data screen written in SQL misses.
		for _, title := range []string{"", " ", "\t", "\n\r", "  ", "\u00a0"} {
			_, err := ts.Lists().Create(ctx, storage.List{Title: title}, owner.ID)
			require.ErrorIs(t, err, storage.ErrInvalidListTitle, "title %q", title)
		}
	})

	t.Run("create stores the trimmed form", func(t *testing.T) {
		created, err := ts.Lists().Create(ctx, storage.List{Title: "  Wedding  "}, owner.ID)
		require.NoError(t, err)
		assert.Equal(t, "Wedding", created.Title)

		// Read it back rather than trusting the returned struct: Create does not
		// re-read, so the value in hand and the value in the row are two facts.
		got, err := ts.Lists().Get(ctx, created.ID)
		require.NoError(t, err)
		assert.Equal(t, "Wedding", got.Title)
	})

	t.Run("update refuses blank and leaves the row alone", func(t *testing.T) {
		before, err := ts.Lists().Get(ctx, list.ID)
		require.NoError(t, err)

		bad := before
		bad.Title = "   "
		_, err = ts.Lists().Update(ctx, bad)
		require.ErrorIs(t, err, storage.ErrInvalidListTitle)

		after, err := ts.Lists().Get(ctx, list.ID)
		require.NoError(t, err)
		assert.Equal(t, before.Title, after.Title)
	})

	t.Run("update stores the trimmed form", func(t *testing.T) {
		l, err := ts.Lists().Get(ctx, list.ID)
		require.NoError(t, err)
		l.Title = "  Renamed  "
		updated, err := ts.Lists().Update(ctx, l)
		require.NoError(t, err)
		assert.Equal(t, "Renamed", updated.Title)
	})
}
