package api_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/api/gen"
)

// #406: a list title was validated nowhere but the two web forms, and they
// disagreed — creation accepted a whitespace-only title that the settings form
// would then refuse to re-save. The rule now lives in the backend, so what these
// cases assert is that an API client, which never touches either form, gets the
// same answer at both surfaces.
func TestListTitle_BlankRejectedAtBothSurfaces(t *testing.T) {
	h := newHarness(t)
	host, tok := h.ownerHost(), h.ownerToken()

	t.Run("create refuses blank and whitespace-only", func(t *testing.T) {
		// The last two entries are non-breaking spaces (U+00A0), written as escapes so
		// they are visible in source. They are the case that separates this rule from a
		// SQL btrim, whose default set does not include them: unicode.IsSpace does. A row
		// screened only by btrim reads clean and is still refused here.
		for _, title := range []string{"", "   ", "\t\n", " ", "\u00a0", "\u00a0\u00a0"} {
			resp, body := h.req(http.MethodPost, "/api/v1/lists", host, tok,
				gen.ListCreate{Title: title})
			require.Equal(t, http.StatusBadRequest, resp.StatusCode,
				"title %q should be refused; body: %s", title, body)
		}
	})

	t.Run("create trims rather than storing padding", func(t *testing.T) {
		resp, body := h.req(http.MethodPost, "/api/v1/lists", host, tok,
			gen.ListCreate{Title: "  Birthday  "})
		require.Equal(t, http.StatusCreated, resp.StatusCode, "body: %s", body)
		assert.Equal(t, "Birthday", *decode[gen.List](t, body).Title)
	})

	t.Run("update refuses blank and leaves the stored title alone", func(t *testing.T) {
		resp, body := h.req(http.MethodPost, "/api/v1/lists", host, tok,
			gen.ListCreate{Title: "Keep me"})
		require.Equal(t, http.StatusCreated, resp.StatusCode, "body: %s", body)
		listID := *decode[gen.List](t, body).Id

		resp, body = h.req(http.MethodPatch, "/api/v1/lists/"+listID, host, tok,
			gen.ListUpdate{Title: ptr("   ")})
		require.Equal(t, http.StatusBadRequest, resp.StatusCode, "body: %s", body)

		// The refusal has to leave the row as it was, not half-apply the patch.
		resp, body = h.req(http.MethodGet, "/api/v1/lists/"+listID, host, tok, nil)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "Keep me", *decode[gen.List](t, body).Title)
	})

	t.Run("update trims", func(t *testing.T) {
		resp, body := h.req(http.MethodPost, "/api/v1/lists", host, tok,
			gen.ListCreate{Title: "Before"})
		require.Equal(t, http.StatusCreated, resp.StatusCode, "body: %s", body)
		listID := *decode[gen.List](t, body).Id

		resp, body = h.req(http.MethodPatch, "/api/v1/lists/"+listID, host, tok,
			gen.ListUpdate{Title: ptr("  After  ")})
		require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
		assert.Equal(t, "After", *decode[gen.List](t, body).Title)
	})

	// An absent title still means "leave it alone" — the three-state merge-patch
	// behaviour the rename surface depends on, unchanged by adding the rule.
	t.Run("update without a title is unaffected", func(t *testing.T) {
		resp, body := h.req(http.MethodPost, "/api/v1/lists", host, tok,
			gen.ListCreate{Title: "Untouched"})
		require.Equal(t, http.StatusCreated, resp.StatusCode, "body: %s", body)
		listID := *decode[gen.List](t, body).Id

		resp, body = h.req(http.MethodPatch, "/api/v1/lists/"+listID, host, tok,
			gen.ListUpdate{Description: ptr("just a description")})
		require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
		assert.Equal(t, "Untouched", *decode[gen.List](t, body).Title)
	})
}
