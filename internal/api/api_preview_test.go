package api_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/api/gen"
)

const ogFixture = `<html><head>
<meta property="og:title" content="Nice Headphones">
<meta property="og:image" content="https://cdn.example/hp.jpg">
<meta property="product:price:amount" content="199.90">
<meta property="product:price:currency" content="EUR">
</head><body>hi</body></html>`

func TestPreviewItem_OK(t *testing.T) {
	h := newHarness(t)
	h.preview.Body = []byte(ogFixture)

	resp, body := h.req(http.MethodPost, "/api/v1/item-previews", h.ownerHost(), h.ownerToken(),
		map[string]any{"url": "https://shop.example/headphones"})
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)

	draft := decode[gen.ItemDraft](t, body)
	require.NotNil(t, draft.Name)
	assert.Equal(t, "Nice Headphones", *draft.Name)
	require.NotNil(t, draft.ImageUrl)
	assert.Equal(t, "https://cdn.example/hp.jpg", *draft.ImageUrl)
	require.NotNil(t, draft.Price)
	assert.Equal(t, 19990, draft.Price.AmountMinor)
	assert.Equal(t, "EUR", draft.Price.Currency)
	// The URL is echoed unchanged (no affiliate rewriting).
	require.NotNil(t, draft.Url)
	assert.Equal(t, "https://shop.example/headphones", *draft.Url)
}

func TestPreviewItem_EmptyPageIs422(t *testing.T) {
	h := newHarness(t)
	h.preview.Body = []byte(`<html><head></head><body>nothing useful</body></html>`)

	resp, _ := h.req(http.MethodPost, "/api/v1/item-previews", h.ownerHost(), h.ownerToken(),
		map[string]any{"url": "https://shop.example/x"})
	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
	assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))
}

func TestPreviewItem_RequiresOwnerAuth(t *testing.T) {
	h := newHarness(t)
	resp, _ := h.req(http.MethodPost, "/api/v1/item-previews", h.ownerHost(), "",
		map[string]any{"url": "https://shop.example/x"})
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestPreviewItem_BadURLIs422(t *testing.T) {
	h := newHarness(t)
	// A non-http scheme never reaches the fetcher.
	resp, _ := h.req(http.MethodPost, "/api/v1/item-previews", h.ownerHost(), h.ownerToken(),
		map[string]any{"url": "file:///etc/passwd"})
	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
}
