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
	// acceptLanguage keeps titles and prices in a predictable language rather
	// than whatever the egress IP's geo suggests.
	acceptLanguage = "en;q=0.9,*;q=0.5"
)

// Fetcher retrieves the HTML body at a URL. The production implementation is
// SSRF-guarded; tests inject a FakeFetcher serving fixture HTML.
type Fetcher interface {
	Fetch(ctx context.Context, rawURL string) ([]byte, error)
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

func (f *SafeFetcher) Fetch(ctx context.Context, rawURL string) ([]byte, error) {
	// Both a context deadline and the client timeout, so a slow-loris body cannot
	// hang past the cap.
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	setPreviewHeaders(req)

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("preview: upstream status %d", resp.StatusCode)
	}
	return readBody(resp)
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
	req.Header.Set("Accept-Language", acceptLanguage)
	req.Header.Set("Accept-Encoding", "identity")
}

// readBody reads at most maxBodyBytes of the response.
//
// Setting Accept-Encoding by hand switches off the transport's transparent
// decompression, so a server that compresses regardless of what we asked for
// would otherwise hand the parser gzip bytes and yield an empty draft. Trust the
// response's own Content-Encoding rather than the request's, and cap both the
// compressed and the decompressed side so a small body cannot expand without
// bound.
func readBody(resp *http.Response) ([]byte, error) {
	r := io.LimitReader(resp.Body, maxBodyBytes)

	if strings.EqualFold(strings.TrimSpace(resp.Header.Get("Content-Encoding")), "gzip") {
		zr, err := gzip.NewReader(r)
		if err != nil {
			return nil, err
		}
		defer func() { _ = zr.Close() }()
		r = io.LimitReader(zr, maxBodyBytes)
	}

	body, err := io.ReadAll(r)
	// A truncated tail is expected whenever the cap cuts a compressed stream
	// short. The head of the document is where the metadata lives, so keep what
	// arrived instead of discarding a usable page.
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, err
	}
	return body, nil
}

func allowedScheme(s string) bool { return s == "http" || s == "https" }

// FakeFetcher is a test double: it returns the configured body/err without any
// network access.
type FakeFetcher struct {
	Body []byte
	Err  error
}

func (f *FakeFetcher) Fetch(context.Context, string) ([]byte, error) {
	return f.Body, f.Err
}
