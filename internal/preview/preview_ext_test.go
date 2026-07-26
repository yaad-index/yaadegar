package preview_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/preview"
)

func run(t *testing.T, body string) (preview.Draft, error) {
	t.Helper()
	p := preview.New(&preview.FakeFetcher{Body: []byte(body)})
	return p.Preview(context.Background(), "https://shop.example/item")
}

func TestExtract_JSONLD(t *testing.T) {
	d, err := run(t, `<html><head>
<script type="application/ld+json">
{"@context":"https://schema.org","@type":"Product","name":"LD Widget",
 "image":"https://cdn.example/ld.jpg",
 "offers":{"@type":"Offer","price":"49.99","priceCurrency":"usd"}}
</script></head></html>`)
	require.NoError(t, err)
	require.NotNil(t, d.Name)
	assert.Equal(t, "LD Widget", *d.Name)
	assert.Equal(t, "https://cdn.example/ld.jpg", *d.ImageURL)
	require.NotNil(t, d.Price)
	assert.Equal(t, int64(4999), d.Price.AmountMinor)
	assert.Equal(t, "USD", d.Price.Currency) // normalized upper-case
	assert.Equal(t, "https://shop.example/item", *d.URL)
}

func TestExtract_OpenGraph(t *testing.T) {
	d, err := run(t, `<html><head>
<meta property="og:title" content="OG Widget">
<meta property="og:image" content="https://cdn.example/og.jpg">
<meta property="product:price:amount" content="12.00">
<meta property="product:price:currency" content="EUR">
</head></html>`)
	require.NoError(t, err)
	assert.Equal(t, "OG Widget", *d.Name)
	assert.Equal(t, "https://cdn.example/og.jpg", *d.ImageURL)
	require.NotNil(t, d.Price)
	assert.Equal(t, int64(1200), d.Price.AmountMinor)
}

func TestExtract_TwitterAndTitleFallback(t *testing.T) {
	d, err := run(t, `<html><head>
<meta name="twitter:image" content="https://cdn.example/tw.jpg">
<title>Just The Title</title>
</head></html>`)
	require.NoError(t, err)
	// No JSON-LD/OG/twitter title → falls back to <title>.
	assert.Equal(t, "Just The Title", *d.Name)
	assert.Equal(t, "https://cdn.example/tw.jpg", *d.ImageURL)
	assert.Nil(t, d.Price)
}

func TestExtract_Precedence(t *testing.T) {
	// JSON-LD name outranks og:title.
	d, err := run(t, `<html><head>
<meta property="og:title" content="OG Name">
<script type="application/ld+json">{"@type":"Product","name":"LD Name"}</script>
</head></html>`)
	require.NoError(t, err)
	assert.Equal(t, "LD Name", *d.Name)
}

func TestExtract_AmbiguousPriceIsNil(t *testing.T) {
	// Unparseable amount → no price (no-price beats wrong-price).
	d, err := run(t, `<html><head>
<meta property="og:title" content="X">
<meta property="product:price:amount" content="call for pricing">
<meta property="product:price:currency" content="EUR">
</head></html>`)
	require.NoError(t, err)
	assert.Nil(t, d.Price)

	// Amount without a currency → no price.
	d2, err := run(t, `<html><head>
<meta property="og:title" content="X">
<meta property="product:price:amount" content="9.99">
</head></html>`)
	require.NoError(t, err)
	assert.Nil(t, d2.Price)
}

func TestExtract_EmptyIsUnfetchable(t *testing.T) {
	_, err := run(t, `<html><head></head><body>nothing</body></html>`)
	assert.ErrorIs(t, err, preview.ErrUnfetchable)
}

func TestPreview_RejectsNonHTTPScheme(t *testing.T) {
	p := preview.New(&preview.FakeFetcher{Body: []byte("<title>x</title>")})
	_, err := p.Preview(context.Background(), "file:///etc/passwd")
	assert.ErrorIs(t, err, preview.ErrUnfetchable)
}

// TestSafeFetcher_BlocksLoopback proves the real guard against a real local
// server: httptest listens on loopback, and the SSRF dial guard refuses it.
func TestSafeFetcher_BlocksLoopback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<title>should never be read</title>"))
	}))
	defer srv.Close()

	_, err := preview.NewSafeFetcher().Fetch(context.Background(), srv.URL)
	require.Error(t, err, "the fetcher must refuse to connect to a loopback address")
}
