package api_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/api/gen"
)

// patchDescription PATCHes the list description (#143) and returns the raw response.
func (h *harness) patchDescription(t *testing.T, listID, desc string) (*http.Response, []byte) {
	t.Helper()
	return h.req(http.MethodPatch, "/api/v1/lists/"+listID, h.ownerHost(), h.ownerToken(),
		map[string]any{"description": desc})
}

func TestListDescriptionRoundTrip(t *testing.T) {
	h := newHarness(t)
	list := h.createList("L")
	require.NotNil(t, list.Description)
	assert.Equal(t, "", *list.Description, "a new list has no description")

	desc := "Gifts for the housewarming — **theme:** plants."
	resp, body := h.patchDescription(t, *list.Id, desc)
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	assert.Equal(t, desc, *decode[gen.List](t, body).Description)

	// Persisted on the owner read.
	resp, body = h.req(http.MethodGet, "/api/v1/lists/"+*list.Id, h.ownerHost(), h.ownerToken(), nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, desc, *decode[gen.List](t, body).Description)

	// And exposed on the public giver read.
	_, pubBody := h.req(http.MethodGet, "/public/"+*list.ShareSlug, h.ownerHost(), "", nil)
	pub := decode[gen.PublicList](t, pubBody)
	require.NotNil(t, pub.Description)
	assert.Equal(t, desc, *pub.Description)
}

func TestListDescriptionClearedWithEmpty(t *testing.T) {
	h := newHarness(t)
	list := h.createList("L")
	h.patchDescription(t, *list.Id, "temporary")
	resp, body := h.patchDescription(t, *list.Id, "")
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	assert.Equal(t, "", *decode[gen.List](t, body).Description)
}

func TestListDescriptionTooLongRejected(t *testing.T) {
	h := newHarness(t)
	list := h.createList("L")

	// The cap is 2000 runes: exactly 2000 is accepted, 2001 is a 400.
	resp, body := h.patchDescription(t, *list.Id, strings.Repeat("x", 2000))
	require.Equal(t, http.StatusOK, resp.StatusCode, "2000 runes accepted; body: %s", body)

	resp, _ = h.patchDescription(t, *list.Id, strings.Repeat("x", 2001))
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "2001 runes rejected")
}
