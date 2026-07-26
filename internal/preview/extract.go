package preview

import (
	"bytes"
	"encoding/json"
	"math"
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

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch strings.ToLower(n.Data) {
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
	d.ImageURL = firstNonEmpty(ldImage, meta["og:image"], meta["og:image:url"], meta["twitter:image"])

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
