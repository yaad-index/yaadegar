package preview

import (
	"bytes"
	"encoding/json"
	"math"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/net/html"

	"github.com/yaad-index/yaadegar/internal/storage"
)

// extract parses HTML and builds a Draft by precedence: JSON-LD (schema.org
// Product/Offer) → OpenGraph → Twitter card → <title>. The source URL is echoed
// unchanged. All fields are optional.
func extract(body []byte, sourceURL string) Draft {
	d := Draft{URL: strp(sourceURL)}

	root, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return d
	}

	meta := map[string]string{} // og:*/twitter:*/product:* → content (first wins)
	var title string
	var ldjson []string
	var bestImg imgCandidate

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch strings.ToLower(n.Data) {
			case "img":
				if c := largestImgCandidate(n); c.betterThan(bestImg) {
					bestImg = c
				}
			case "meta":
				key := attr(n, "property")
				if key == "" {
					key = attr(n, "name")
				}
				content := attr(n, "content")
				if key != "" && content != "" {
					k := strings.ToLower(key)
					if _, seen := meta[k]; !seen {
						meta[k] = content
					}
				}
			case "title":
				if title == "" && n.FirstChild != nil && n.FirstChild.Type == html.TextNode {
					title = strings.TrimSpace(n.FirstChild.Data)
				}
			case "script":
				if strings.EqualFold(attr(n, "type"), "application/ld+json") && n.FirstChild != nil {
					ldjson = append(ldjson, n.FirstChild.Data)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)

	ldName, ldImage, ldAmount, ldCurrency := parseLDJSON(ldjson)

	d.Name = firstNonEmpty(ldName, meta["og:title"], meta["twitter:title"], title)
	// The DOM candidate is deliberately last: a page that publishes a social-card
	// image keeps using it, and the crawl only matters for pages that publish none.
	d.ImageURL = firstNonEmpty(ldImage, meta["og:image"], meta["og:image:url"], meta["twitter:image"],
		absoluteImageURL(bestImg.url, sourceURL))

	amount, currency := ldAmount, ldCurrency
	if amount == "" {
		amount, currency = meta["product:price:amount"], meta["product:price:currency"]
	}
	if amount == "" {
		amount, currency = meta["og:price:amount"], meta["og:price:currency"]
	}
	d.Price = parsePrice(amount, currency)

	return d
}

// minImageDim is the floor below which an <img> is treated as chrome — an icon,
// a spacer, a rating star — rather than a picture of the product.
const minImageDim = 200

// imgCandidate is a product-image guess taken from the DOM, kept with its area so
// competing candidates on the same page can be ranked.
type imgCandidate struct {
	url  string
	area int
}

func (c imgCandidate) betterThan(other imgCandidate) bool {
	return c.url != "" && c.area > other.area
}

// largestImgCandidate scores a single <img>.
//
// Some retailers publish no og:image, no twitter:image and no ld+json image, and
// carry the product picture only in the DOM. The richest form is a JSON map of
// URL to [width, height] in an attribute, which names every rendition at once and
// so states its own dimensions; failing that, an element that declares explicit
// width and height is usable. An <img> that declares no size is skipped rather
// than guessed at — an unranked candidate would outrank nothing and could just as
// easily be a banner.
func largestImgCandidate(n *html.Node) imgCandidate {
	var best imgCandidate

	// These attribute names are a single large retailer's own markup convention,
	// not a web standard and not something to generalise from — which is why they
	// are tried first but nothing depends on them, and why the width/height path
	// below stands on its own. Attribute values arrive unescaped from the parser,
	// so the value is plain JSON.
	for _, key := range []string{"data-a-dynamic-image", "data-dynamic-image"} {
		raw := attr(n, key)
		if raw == "" {
			continue
		}
		var byURL map[string][]int
		if err := json.Unmarshal([]byte(raw), &byURL); err != nil {
			continue
		}
		for u, wh := range byURL {
			if len(wh) != 2 || !usableImageURL(u) {
				continue
			}
			if wh[0] < minImageDim || wh[1] < minImageDim {
				continue
			}
			if c := (imgCandidate{url: u, area: wh[0] * wh[1]}); c.betterThan(best) {
				best = c
			}
		}
	}
	if best.url != "" {
		return best
	}

	src := firstNonEmptyString(attr(n, "src"), attr(n, "data-src"))
	if !usableImageURL(src) {
		return imgCandidate{}
	}
	w, errW := strconv.Atoi(strings.TrimSpace(attr(n, "width")))
	h, errH := strconv.Atoi(strings.TrimSpace(attr(n, "height")))
	if errW != nil || errH != nil || w < minImageDim || h < minImageDim {
		return imgCandidate{}
	}
	return imgCandidate{url: src, area: w * h}
}

// usableImageURL rejects what cannot become an image_url worth storing: an empty
// src, and anything carrying a scheme this package will not serve — data:, which
// would embed a whole image in the field, but equally blob:, javascript:, mailto:
// and every other scheme.
//
// The rejection has to happen here, while ranking, and not only at the end where
// absoluteImageURL enforces the same http/https rule. A candidate that wins on
// area and is discarded afterwards takes a smaller, perfectly good image down
// with it, and the page ends up with no picture at all.
//
// A scheme-less src is kept: it is relative (or protocol-relative) and is
// resolved against the page URL later, which is where it gets held to the rule.
func usableImageURL(u string) bool {
	u = strings.TrimSpace(u)
	if u == "" {
		return false
	}
	parsed, err := url.Parse(u)
	if err != nil {
		return false
	}
	return parsed.Scheme == "" || allowedScheme(parsed.Scheme)
}

// absoluteImageURL resolves a DOM-sourced src against the page it came from. A
// relative src is common in markup and useless once stored, and the result is
// held to the same http/https restriction as everything else here.
func absoluteImageURL(raw, sourceURL string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	ref, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if ref.IsAbs() {
		if !allowedScheme(ref.Scheme) {
			return ""
		}
		return ref.String()
	}
	base, err := url.Parse(sourceURL)
	if err != nil || !base.IsAbs() {
		return ""
	}
	abs := base.ResolveReference(ref)
	if !allowedScheme(abs.Scheme) {
		return ""
	}
	return abs.String()
}

func firstNonEmptyString(vals ...string) string {
	for _, v := range vals {
		if t := strings.TrimSpace(v); t != "" {
			return t
		}
	}
	return ""
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, key) {
			return a.Val
		}
	}
	return ""
}

func strp(s string) *string { return &s }

func firstNonEmpty(vals ...string) *string {
	for _, v := range vals {
		if t := strings.TrimSpace(v); t != "" {
			return &t
		}
	}
	return nil
}

// parsePrice converts a scraped amount + currency to Money. It is deliberately
// conservative: an empty currency or an unparseable amount yields nil — for a
// suggestion the user re-types, no-price beats a wrong price. v1 assumes a
// 2-decimal (minor-unit ×100) currency; refining per-currency exponents is a
// later enhancement.
func parsePrice(amount, currency string) *storage.Money {
	currency = strings.ToUpper(strings.TrimSpace(currency))
	amount = strings.TrimSpace(amount)
	if currency == "" || amount == "" {
		return nil
	}
	f, err := strconv.ParseFloat(amount, 64)
	if err != nil || f < 0 || math.IsInf(f, 0) || math.IsNaN(f) {
		return nil
	}
	return &storage.Money{AmountMinor: int64(math.Round(f * 100)), Currency: currency}
}

// --- JSON-LD digging (schema.org Product/Offer) ---

func parseLDJSON(scripts []string) (name, image, amount, currency string) {
	for _, s := range scripts {
		var v any
		if err := json.Unmarshal([]byte(s), &v); err != nil {
			continue
		}
		for _, node := range ldNodes(v) {
			m, ok := node.(map[string]any)
			if !ok || !typeContains(m["@type"], "Product") {
				continue
			}
			if name == "" {
				name = asString(m["name"])
			}
			if image == "" {
				image = asImage(m["image"])
			}
			if amount == "" {
				if a, c := asOffer(m["offers"]); a != "" {
					amount, currency = a, c
				}
			}
		}
	}
	return
}

// ldNodes flattens arrays and @graph containers into a flat list of nodes.
func ldNodes(v any) []any {
	switch t := v.(type) {
	case []any:
		var out []any
		for _, e := range t {
			out = append(out, ldNodes(e)...)
		}
		return out
	case map[string]any:
		if g, ok := t["@graph"]; ok {
			return ldNodes(g)
		}
		return []any{t}
	default:
		return nil
	}
}

func typeContains(v any, want string) bool {
	switch t := v.(type) {
	case string:
		return strings.EqualFold(t, want)
	case []any:
		for _, e := range t {
			if s, ok := e.(string); ok && strings.EqualFold(s, want) {
				return true
			}
		}
	}
	return false
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

// asImage accepts a string, an array (first usable), or an object with a url.
func asImage(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case []any:
		for _, e := range t {
			if s := asImage(e); s != "" {
				return s
			}
		}
	case map[string]any:
		return asString(t["url"])
	}
	return ""
}

// asOffer extracts price + priceCurrency from an Offer (object or array). Price
// may be a string or a number.
func asOffer(v any) (amount, currency string) {
	switch t := v.(type) {
	case []any:
		for _, e := range t {
			if a, c := asOffer(e); a != "" {
				return a, c
			}
		}
	case map[string]any:
		return numOrString(t["price"]), asString(t["priceCurrency"])
	}
	return "", ""
}

func numOrString(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	}
	return ""
}
