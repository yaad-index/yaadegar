package preview

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	maxRedirects = 3
	maxBodyBytes = 512 * 1024 // read at most ~512 KB of HTML
	fetchTimeout = 8 * time.Second
	dialTimeout  = 5 * time.Second
	userAgent    = "yaadegar-link-preview/1.0 (+https://github.com/yaad-index/yaadegar)"

	// acceptHTML mirrors what a browser asks for. Some retailers vary their
	// response on it and serve a reduced page to a bare Accept.
	acceptHTML = "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"
)

// Page is a fetched document: the body, plus the URL that actually served it.
//
// The two differ whenever the request redirected, which is the normal case for a
// shortened or tracking link. Callers that reason about *who answered* — rather
// than who was asked — must use FinalURL, or they draw conclusions about a host
// that served nothing.
type Page struct {
	Body []byte
	// FinalURL is the URL the response came from, after any redirects. A fetcher
	// that does not track redirects may leave it empty, and the caller then falls
	// back to the requested URL.
	FinalURL string
}

// Fetcher retrieves the HTML at a URL. The production implementation is
// SSRF-guarded; tests inject a FakeFetcher serving fixture HTML.
type Fetcher interface {
	Fetch(ctx context.Context, rawURL string) (Page, error)
}

// SafeFetcher fetches remote HTML with SSRF protections: a Control-hook dialer
// that rejects non-public IPs on every connection (redirects and DNS rebinding
// included), an http/https-only redirect policy capped low, a strict timeout,
// and a response-size cap.
type SafeFetcher struct{ client *http.Client }

// NewSafeFetcher builds the guarded fetcher.
func NewSafeFetcher() *SafeFetcher {
	dialer := &net.Dialer{Timeout: dialTimeout, Control: dialGuard}
	transport := &http.Transport{
		DialContext:           dialer.DialContext,
		Proxy:                 nil, // never route through an env proxy (would bypass the guard)
		TLSHandshakeTimeout:   dialTimeout,
		ResponseHeaderTimeout: fetchTimeout,
		DisableKeepAlives:     true,
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   fetchTimeout, // overall cap, alongside the per-request context deadline
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("preview: too many redirects")
			}
			if !allowedScheme(req.URL.Scheme) {
				return fmt.Errorf("preview: disallowed redirect scheme %q", req.URL.Scheme)
			}
			return nil
		},
	}
	return &SafeFetcher{client: client}
}

func (f *SafeFetcher) Fetch(ctx context.Context, rawURL string) (Page, error) {
	// Both a context deadline and the client timeout, so a slow-loris body cannot
	// hang past the cap.
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return Page{}, err
	}
	setPreviewHeaders(req)

	resp, err := f.client.Do(req)
	if err != nil {
		return Page{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return Page{}, fmt.Errorf("preview: upstream status %d", resp.StatusCode)
	}
	body, err := readBody(resp)
	if err != nil {
		return Page{}, err
	}
	return Page{Body: body, FinalURL: servedURL(resp, rawURL)}, nil
}

// servedURL reports the URL that actually produced resp.
//
// net/http sets Response.Request to the LAST request in the redirect chain, not
// the one handed to Do, which is the only reason the serving host is recoverable
// after a shortened link resolves. Falls back to the requested URL so a response
// carrying no request (a hand-built one, or a client that does not populate it)
// degrades to the old behaviour rather than to an empty host.
func servedURL(resp *http.Response, requested string) string {
	if resp != nil && resp.Request != nil && resp.Request.URL != nil {
		return resp.Request.URL.String()
	}
	return requested
}

// setPreviewHeaders pins every negotiated header explicitly instead of letting
// the transport pick.
//
// Accept-Encoding matters most. http.Transport adds "gzip" on its own whenever
// the caller leaves the header unset, and at least one major retailer varies its
// response on that header — serving an anti-automation stub to the compressed
// request and the real product page to the plain one. Asking for identity keeps
// us on the page a browser would get. The extra bandwidth is already bounded by
// maxBodyBytes.
//
// It has to be set to "identity" rather than merely left off: a server that sees
// no Accept-Encoding at all is free to compress anyway, and observed behaviour
// is that one does.
func setPreviewHeaders(req *http.Request) {
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", acceptHTML)
	req.Header.Set("Accept-Encoding", "identity")
}

// readBody reads at most maxBodyBytes of the response.
//
// Setting Accept-Encoding by hand switches off the transport's transparent
// decompression, so a server that compresses regardless of what we asked for
// would otherwise hand the parser gzip bytes and yield an empty draft. Trust the
// response's own Content-Encoding rather than the request's.
func readBody(resp *http.Response) ([]byte, error) {
	r := io.LimitReader(resp.Body, maxBodyBytes)

	switch enc := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Encoding"))); enc {
	case "", "identity":
		// Nothing is tolerated on this path. The size cap cannot produce a short
		// read here — io.LimitReader signals its limit with a plain io.EOF, which
		// io.ReadAll already reports as success — so an unexpected EOF means the
		// connection dropped mid-download, and returning the partial HTML would
		// be a silent success built on an incomplete page.
		body, err := io.ReadAll(r)
		if err != nil {
			return nil, err
		}
		return body, nil

	case "gzip":
		zr, err := gzip.NewReader(r)
		if err != nil {
			return nil, err
		}
		defer func() { _ = zr.Close() }()

		// Cap the decompressed side too, so a small body cannot expand without
		// bound.
		body, err := io.ReadAll(io.LimitReader(zr, maxBodyBytes))
		// Only here is a truncated tail expected rather than exceptional: the cap
		// can cut the compressed stream mid-way. The metadata sits in the head of
		// the document, so keep what arrived.
		if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, err
		}
		return body, nil

	default:
		// br, deflate, anything else. Without this the body reaches the parser as
		// binary and fails as "no metadata found", which is indistinguishable
		// from a page that genuinely publishes none.
		return nil, fmt.Errorf("preview: unsupported content encoding %q", enc)
	}
}

func allowedScheme(s string) bool { return s == "http" || s == "https" }

// FakeFetcher is a test double: it returns the configured body/err without any
// network access. FinalURL stands in for a redirect chain — leaving it empty
// means "served by the URL that was requested".
type FakeFetcher struct {
	Body     []byte
	FinalURL string
	Err      error
}

func (f *FakeFetcher) Fetch(context.Context, string) (Page, error) {
	return Page{Body: f.Body, FinalURL: f.FinalURL}, f.Err
}
