// Package preview implements the browser "auto-add from a product URL" endpoint's
// core: fetch a user-supplied page server-side and extract a best-effort item
// draft. Because it fetches an arbitrary URL on a multi-tenant host, the fetch is
// SSRF-guarded at the socket layer (see ipguard.go / fetcher.go). Nothing here
// creates an item; the client reviews the draft and posts it to /items.
package preview

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"github.com/yaad-index/yaadegar/internal/storage"
)

// Draft is a best-effort scraped item. Every field is optional; the URL echoes
// the input unchanged (never rewritten — no affiliate params). The price is a
// suggestion only, and the client keeps it editable.
type Draft struct {
	Name     *string
	URL      *string
	ImageURL *string
	Price    *storage.Money
}

// hasContent reports whether the draft found anything worth returning (the echoed
// URL alone does not count).
func (d Draft) hasContent() bool {
	return d.Name != nil || d.ImageURL != nil || d.Price != nil
}

// ErrUnfetchable means the URL could not be fetched or yielded nothing usable —
// the caller should fall back to manual entry (HTTP 422).
var ErrUnfetchable = errors.New("preview: could not fetch or parse a usable preview")

// Previewer orchestrates fetch + extract. The Fetcher is injected so tests serve
// fixtures with no network.
type Previewer struct{ fetcher Fetcher }

// New builds a Previewer over a Fetcher.
func New(f Fetcher) *Previewer { return &Previewer{fetcher: f} }

// NewDefault builds a Previewer backed by the SSRF-guarded SafeFetcher.
func NewDefault() *Previewer { return New(NewSafeFetcher()) }

// Preview fetches rawURL and returns a draft, or ErrUnfetchable on any failure or
// an empty result. Scheme is validated up front (defence in depth alongside the
// fetcher's own guard).
func (p *Previewer) Preview(ctx context.Context, rawURL string) (Draft, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Host == "" || !allowedScheme(u.Scheme) {
		return Draft{}, ErrUnfetchable
	}
	body, err := p.fetcher.Fetch(ctx, rawURL)
	if err != nil {
		return Draft{}, ErrUnfetchable
	}
	d := extract(body, rawURL)
	if !d.hasContent() || namesTheSiteItself(d.Name, u.Host) {
		return Draft{}, ErrUnfetchable
	}
	return d, nil
}

// namesTheSiteItself reports whether the only thing scraped was the site's own
// name — "Acme.example" for a page on acme.example.
//
// An anti-automation stub answers 200 with a short body whose title is the site
// name, so neither the status code nor hasContent distinguishes a fetched page
// from a refused one. Without this check the refusal is the failure that looks
// like a success: the user gets an item named after a shop and no signal that
// anything went wrong. Failing here routes them to manual entry instead.
//
// Deliberately an exact match on the host or its first label, not a prefix or
// substring test — a real product legitimately called "Acme Basics Cable" must
// still preview.
func namesTheSiteItself(name *string, host string) bool {
	if name == nil {
		return false
	}
	n := strings.ToLower(strings.TrimSpace(*name))
	if n == "" {
		return false
	}
	h := strings.ToLower(host)
	if i := strings.IndexByte(h, ':'); i >= 0 { // drop any port
		h = h[:i]
	}
	h = strings.TrimPrefix(h, "www.")
	if h == "" {
		return false
	}
	return n == h || n == strings.SplitN(h, ".", 2)[0]
}
