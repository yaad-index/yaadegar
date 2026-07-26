package preview

import (
	"fmt"
	"net"
	"syscall"
)

// cgnat is the 100.64.0.0/10 carrier-grade NAT range, which the stdlib
// IsPrivate does not cover.
var cgnat = &net.IPNet{IP: net.IPv4(100, 64, 0, 0).To4(), Mask: net.CIDRMask(10, 32)}

// blockedIP reports whether an IP must not be dialed — the SSRF guard. It fails
// closed: a nil or unclassifiable address is blocked. IPv4-mapped IPv6
// (::ffff:a.b.c.d) is unwrapped first so the classifiers and the CGNAT check
// apply to the real IPv4 (e.g. ::ffff:10.0.0.1 is caught as private).
func blockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	if !ip.IsGlobalUnicast() ||
		ip.IsLoopback() ||
		ip.IsPrivate() || // 10/8, 172.16/12, 192.168/16, fc00::/7
		ip.IsLinkLocalUnicast() || // 169.254/16 (incl. 169.254.169.254), fe80::/10
		ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() ||
		ip.IsMulticast() {
		return true
	}
	if ip.To4() != nil && cgnat.Contains(ip) {
		return true
	}
	return false
}

// dialGuard is the net.Dialer.Control hook: it runs on every connection with the
// actual resolved dial address, and rejects any non-public target. Because it
// sees the resolved IP per-connection, it catches redirects and DNS rebinding
// (TOCTOU) that an up-front hostname check would miss. Parse failures fail closed.
func dialGuard(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("preview: unparseable dial address %q", address)
	}
	ip := net.ParseIP(host)
	if ip == nil || blockedIP(ip) {
		return fmt.Errorf("preview: refusing to connect to non-public address %q", host)
	}
	return nil
}
