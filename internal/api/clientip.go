package api

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// ParseTrustedProxies parses the configured trusted-proxy entries: single
// addresses ("10.0.0.5", "fd00::1") or CIDR ranges ("10.0.0.0/24"). An empty or
// nil input yields an empty list, which trusts no proxy.
func ParseTrustedProxies(entries []string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, e := range entries {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		if strings.Contains(e, "/") {
			p, err := netip.ParsePrefix(e)
			if err != nil {
				return nil, fmt.Errorf("trusted proxy %q: %w", e, err)
			}
			out = append(out, p.Masked())
			continue
		}
		a, err := netip.ParseAddr(e)
		if err != nil {
			return nil, fmt.Errorf("trusted proxy %q: %w", e, err)
		}
		a = a.Unmap()
		out = append(out, netip.PrefixFrom(a, a.BitLen()))
	}
	return out, nil
}

// trusted reports whether addr is one of the configured proxies.
func trusted(addr netip.Addr, proxies []netip.Prefix) bool {
	addr = addr.Unmap()
	for _, p := range proxies {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// resolveClientIP returns the address rate limits key on. The request's direct
// peer is the answer unless that peer is a trusted proxy. Only then is
// X-Forwarded-For consulted, walked from the right (the hop nearest this server)
// past every trusted proxy; the first address that is not a trusted proxy is the
// client. When every hop is trusted (a client on the same private network as a
// proxy whose whole range is trusted), the leftmost hop is the client: it is the
// address the outermost trusted proxy saw. A header from an untrusted peer is
// ignored entirely, because anyone can send one.
func resolveClientIP(remoteAddr string, forwardedFor []string, proxies []netip.Prefix) string {
	peer := clientIP(remoteAddr)
	if len(proxies) == 0 {
		return peer
	}
	peerAddr, err := netip.ParseAddr(peer)
	if err != nil || !trusted(peerAddr, proxies) {
		return peer
	}
	var hops []string
	for _, h := range forwardedFor {
		for _, part := range strings.Split(h, ",") {
			if part = strings.TrimSpace(part); part != "" {
				hops = append(hops, part)
			}
		}
	}
	leftmost := ""
	for i := len(hops) - 1; i >= 0; i-- {
		a, err := netip.ParseAddr(hops[i])
		if err != nil {
			// An entry that is not an address cannot be trusted or keyed on; stop
			// at the last hop that was one, which is the trusted peer itself.
			return peer
		}
		if !trusted(a, proxies) {
			return a.Unmap().String()
		}
		leftmost = a.Unmap().String()
	}
	if leftmost != "" {
		return leftmost
	}
	return peer
}

// clientIP strips the port from a RemoteAddr ("host:port" → "host").
func clientIP(remoteAddr string) string {
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
		return host
	}
	return remoteAddr
}

// captureClientIP lifts the client address into the context for rate limiting:
// the direct peer, or the forwarded client when the peer is a trusted proxy.
func (s *Server) captureClientIP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := resolveClientIP(r.RemoteAddr, r.Header.Values("X-Forwarded-For"), s.trustedProxies)
		next.ServeHTTP(w, r.WithContext(withClientIP(r.Context(), ip)))
	})
}
