package preview

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

const (
	maxRedirects = 3
	maxBodyBytes = 512 * 1024 // read at most ~512 KB of HTML
	fetchTimeout = 8 * time.Second
	dialTimeout  = 5 * time.Second
	userAgent    = "yaadegar-link-preview/1.0 (+https://github.com/yaad-index/yaadegar)"
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
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("preview: upstream status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
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
