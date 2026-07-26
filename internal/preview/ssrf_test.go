package preview

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBlockedIP(t *testing.T) {
	blocked := []string{
		"127.0.0.1", "127.1.2.3", // loopback
		"10.0.0.1", "172.16.5.5", "172.31.0.0", "192.168.1.1", // RFC1918
		"169.254.1.1", "169.254.169.254", // link-local + cloud metadata
		"::1",                     // IPv6 loopback
		"fc00::1", "fd12:3456::1", // ULA
		"fe80::1",       // IPv6 link-local
		"0.0.0.0", "::", // unspecified
		"100.64.0.1", "100.127.255.255", // CGNAT 100.64/10
		"::ffff:10.0.0.1", "::ffff:127.0.0.1", // IPv4-mapped IPv6 must unwrap
		"224.0.0.1", "239.1.1.1", // multicast
		"255.255.255.255", // limited broadcast (not global unicast)
	}
	for _, s := range blocked {
		assert.True(t, blockedIP(net.ParseIP(s)), "expected %s blocked", s)
	}

	allowed := []string{
		"8.8.8.8", "1.1.1.1", "93.184.216.34",
		"2606:2800:220:1:248:1893:25c8:1946", // example.com AAAA
	}
	for _, s := range allowed {
		assert.False(t, blockedIP(net.ParseIP(s)), "expected %s allowed", s)
	}

	assert.True(t, blockedIP(nil), "nil address fails closed")
}

func TestDialGuard(t *testing.T) {
	assert.Error(t, dialGuard("tcp4", "127.0.0.1:80", nil))
	assert.Error(t, dialGuard("tcp6", "[::1]:80", nil))
	assert.Error(t, dialGuard("tcp4", "10.0.0.5:443", nil))
	assert.Error(t, dialGuard("tcp4", "169.254.169.254:80", nil))
	assert.NoError(t, dialGuard("tcp4", "8.8.8.8:443", nil))
	// Unparseable / non-IP address fails closed.
	assert.Error(t, dialGuard("tcp4", "not-an-address", nil))
	assert.Error(t, dialGuard("tcp4", "example.com:80", nil)) // hostname, not a resolved IP
}
