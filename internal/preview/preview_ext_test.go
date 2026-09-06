package preview_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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

// TestExtract_DOMImageFallback covers a page that publishes no social-card image
// at all and carries the product picture only in the DOM, as a JSON map of URL to
// [width, height]. The largest rendition wins.
func TestExtract_DOMImageFallback(t *testing.T) {
	d, err := run(t, `<html><head><title>Blue Widget 3000</title></head><body>
<img id="landingImage" data-a-dynamic-image='{"https://cdn.example/small.jpg":[355,284],"https://cdn.example/large.jpg":[679,679],"https://cdn.example/mid.jpg":[450,450]}'>
</body></html>`)
	require.NoError(t, err)
	require.NotNil(t, d.ImageURL)
	assert.Equal(t, "https://cdn.example/large.jpg", *d.ImageURL)
}

// TestExtract_MetadataImageOutranksDOM keeps the fallback last: a page that does
// publish og:image must be unaffected by the crawl.
func TestExtract_MetadataImageOutranksDOM(t *testing.T) {
	d, err := run(t, `<html><head>
<meta property="og:title" content="OG Widget">
<meta property="og:image" content="https://cdn.example/og.jpg">
</head><body>
<img data-a-dynamic-image='{"https://cdn.example/dom.jpg":[999,999]}'>
</body></html>`)
	require.NoError(t, err)
	assert.Equal(t, "https://cdn.example/og.jpg", *d.ImageURL)
}

func TestExtract_DOMImageIgnoresChrome(t *testing.T) {
	t.Run("icons below the size floor are skipped", func(t *testing.T) {
		d, err := run(t, `<html><head><title>Widget</title></head><body>
<img src="https://cdn.example/star.png" width="16" height="16">
<img src="https://cdn.example/logo.png" width="120" height="40">
</body></html>`)
		require.NoError(t, err)
		assert.Nil(t, d.ImageURL, "chrome must not become the product image")
	})

	t.Run("an img with no declared size is skipped", func(t *testing.T) {
		d, err := run(t, `<html><head><title>Widget</title></head><body>
<img src="https://cdn.example/unknown.jpg">
</body></html>`)
		require.NoError(t, err)
		assert.Nil(t, d.ImageURL, "an unranked candidate must not win by default")
	})

	t.Run("a data URI is never stored", func(t *testing.T) {
		d, err := run(t, `<html><head><title>Widget</title></head><body>
<img src="data:image/gif;base64,R0lGODlhAQABAAAAACw=" width="600" height="600">
</body></html>`)
		require.NoError(t, err)
		assert.Nil(t, d.ImageURL, "an inline data URI must not become image_url")
	})

	// An unservable scheme must be rejected while ranking, not merely dropped at
	// the end. If it is allowed to win on area, the real image on the page loses
	// to it and is then discarded for having an unusable scheme — costing an image
	// that was there all along.
	//
	// data: is the obvious case, but the rule is about the scheme, not that one
	// prefix, so every other scheme is checked the same way.
	for _, src := range []string{
		"data:image/gif;base64,R0lGODlhAQABAAAAACw=",
		"blob:https://shop.example/2f8a-4c1d",
		"javascript:void(0)",
		"mailto:someone@shop.example",
		"ftp://cdn.example/legacy.jpg",
	} {
		t.Run("a large "+src[:strings.Index(src, ":")]+": src does not displace a smaller real image", func(t *testing.T) {
			d, err := run(t, `<html><head><title>Widget</title></head><body>
<img src="`+src+`" width="900" height="900">
<img src="https://cdn.example/real.jpg" width="300" height="300">
</body></html>`)
			require.NoError(t, err)
			require.NotNil(t, d.ImageURL, "the real image must survive a larger unservable candidate")
			assert.Equal(t, "https://cdn.example/real.jpg", *d.ImageURL)
		})
	}

	t.Run("a protocol-relative src is kept and resolved", func(t *testing.T) {
		d, err := run(t, `<html><head><title>Widget</title></head><body>
<img src="//cdn.example/proto-relative.jpg" width="600" height="600">
</body></html>`)
		require.NoError(t, err)
		require.NotNil(t, d.ImageURL, "a scheme-less src is relative, not unservable")
		assert.Equal(t, "https://cdn.example/proto-relative.jpg", *d.ImageURL)
	})
}

// TestExtract_DOMImageResolvesRelativeSrc — a relative src is common in markup and
// useless once stored, so it is resolved against the page it came from.
func TestExtract_DOMImageResolvesRelativeSrc(t *testing.T) {
	d, err := run(t, `<html><head><title>Widget</title></head><body>
<img src="/media/widget.jpg" width="600" height="600">
</body></html>`)
	require.NoError(t, err)
	require.NotNil(t, d.ImageURL)
	assert.Equal(t, "https://shop.example/media/widget.jpg", *d.ImageURL)
}

// TestExtract_DOMImagePicksLargestAcrossElements — thumbnails and the main image
// are separate elements; the biggest declared picture on the page wins.
func TestExtract_DOMImagePicksLargestAcrossElements(t *testing.T) {
	d, err := run(t, `<html><head><title>Widget</title></head><body>
<img src="https://cdn.example/thumb.jpg" width="220" height="220">
<img src="https://cdn.example/hero.jpg" width="800" height="800">
<img src="https://cdn.example/other.jpg" width="300" height="300">
</body></html>`)
	require.NoError(t, err)
	require.NotNil(t, d.ImageURL)
	assert.Equal(t, "https://cdn.example/hero.jpg", *d.ImageURL)
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
