package api_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/api/gen"
)

// The list-index summary carries item previews for the dashboard card cluster
// (#207): up to the first three items in item display order (priority DESC, then
// oldest first), each with its own image URL or null for an imageless item, plus
// the full item_count so the card can derive the "+N" overflow. A single-list read
// carries no previews.
func TestListLists_ItemPreviews(t *testing.T) {
	h := newHarness(t)
	host, tok := h.ownerHost(), h.ownerToken()

	resp, body := h.req(http.MethodPost, "/api/v1/lists", host, tok, gen.ListCreate{Title: "Previews"})
	require.Equal(t, http.StatusCreated, resp.StatusCode, "body: %s", body)
	listID := *decode[gen.List](t, body).Id

	// Five items whose display order (priority DESC, created_at, id) is deterministic:
	// B(10) first; then C,D at priority 5 by creation order; then A,E at 0. So the top
	// three previews are B (no image), C, D — exercising ordering, the null image, and
	// the three-item cap with two items (A, E) left over for the "+N" overflow.
	add := func(name string, priority int, image *string) string {
		p := priority
		it := gen.ItemCreate{Name: name, Priority: &p, ImageUrl: image}
		resp, body := h.req(http.MethodPost, "/api/v1/lists/"+listID+"/items", host, tok, it)
		require.Equal(t, http.StatusCreated, resp.StatusCode, "body: %s", body)
		return *decode[gen.Item](t, body).Id
	}
	imgC := "https://img.example/c.png"
	imgD := "https://img.example/d.png"
	imgA := "https://img.example/a.png"
	_ = add("A", 0, &imgA)
	bID := add("B", 10, nil)
	cID := add("C", 5, &imgC)
	dID := add("D", 5, &imgD)
	_ = add("E", 0, nil)

	// A second, empty list proves previews are omitted (not an empty array of noise)
	// when there is nothing to preview.
	resp, body = h.req(http.MethodPost, "/api/v1/lists", host, tok, gen.ListCreate{Title: "Empty"})
	require.Equal(t, http.StatusCreated, resp.StatusCode, "body: %s", body)
	emptyID := *decode[gen.List](t, body).Id

	resp, body = h.req(http.MethodGet, "/api/v1/lists", host, tok, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	page := decode[gen.ListPage](t, body)
	require.NotNil(t, page.Items)

	byID := map[string]gen.List{}
	for _, l := range *page.Items {
		byID[*l.Id] = l
	}

	withItems := byID[listID]
	require.NotNil(t, withItems.ItemCount)
	assert.Equal(t, 5, *withItems.ItemCount, "the count is the full total, not the previewed subset")
	require.NotNil(t, withItems.ItemPreviews, "the summary must carry the preview cluster")
	previews := *withItems.ItemPreviews
	require.Len(t, previews, 3, "previews are capped at three")

	// Order matches the item display order, and the first (B) has no image of its own.
	assert.Equal(t, bID, *previews[0].Id)
	assert.Nil(t, previews[0].ImageUrl, "an imageless item previews with a null image, not an omitted entry")
	assert.Equal(t, cID, *previews[1].Id)
	require.NotNil(t, previews[1].ImageUrl)
	assert.Equal(t, imgC, *previews[1].ImageUrl)
	assert.Equal(t, dID, *previews[2].Id)
	require.NotNil(t, previews[2].ImageUrl)
	assert.Equal(t, imgD, *previews[2].ImageUrl)

	// The empty list carries no previews.
	empty := byID[emptyID]
	require.NotNil(t, empty.ItemCount)
	assert.Equal(t, 0, *empty.ItemCount)
	assert.Nil(t, empty.ItemPreviews, "an empty list previews nothing")

	// A single-list read is not the summary and carries no cluster.
	resp, body = h.req(http.MethodGet, "/api/v1/lists/"+listID, host, tok, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	one := decode[gen.List](t, body)
	assert.Nil(t, one.ItemPreviews, "GetList is not the index summary; it carries no previews")
}
