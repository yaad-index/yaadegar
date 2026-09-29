package api

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTrustedProxies(t *testing.T) {
	got, err := ParseTrustedProxies([]string{"10.0.0.5", " 10.1.0.0/16 ", "", "fd00::1"})
	require.NoError(t, err)
	assert.Equal(t, []netip.Prefix{
		netip.MustParsePrefix("10.0.0.5/32"),
		netip.MustParsePrefix("10.1.0.0/16"),
		netip.MustParsePrefix("fd00::1/128"),
	}, got)

	none, err := ParseTrustedProxies(nil)
	require.NoError(t, err)
	assert.Empty(t, none)

	for _, bad := range []string{"not-an-ip", "10.0.0.0/99", "10.0.0.1:80"} {
		_, err := ParseTrustedProxies([]string{bad})
		assert.Error(t, err, bad)
	}
}

func TestResolveClientIP(t *testing.T) {
	proxies, err := ParseTrustedProxies([]string{"10.0.0.1", "172.16.0.0/12"})
	require.NoError(t, err)

	cases := []struct {
		name    string
		remote  string
		xff     []string
		proxies []netip.Prefix
		want    string
	}{
		{"no proxies configured: peer, header ignored", "10.0.0.1:5000", []string{"203.0.113.5"}, nil, "10.0.0.1"},
		{"untrusted peer: header ignored", "198.51.100.7:5000", []string{"203.0.113.5"}, proxies, "198.51.100.7"},
		{"trusted peer, one hop", "10.0.0.1:5000", []string{"203.0.113.5"}, proxies, "203.0.113.5"},
		{"trusted peer, no header: peer", "10.0.0.1:5000", nil, proxies, "10.0.0.1"},
		{"walks past trusted hops from the right", "10.0.0.1:5000", []string{"203.0.113.5, 172.16.4.2"}, proxies, "203.0.113.5"},
		{"a client-supplied left entry is not believed", "10.0.0.1:5000", []string{"1.2.3.4, 203.0.113.5"}, proxies, "203.0.113.5"},
		{"several headers are one list", "10.0.0.1:5000", []string{"203.0.113.5", "172.16.4.2"}, proxies, "203.0.113.5"},
		{"garbage hop stops at the peer", "10.0.0.1:5000", []string{"not-an-ip"}, proxies, "10.0.0.1"},
		{"all hops trusted: the leftmost hop is the client", "10.0.0.1:5000", []string{"172.16.4.2"}, proxies, "172.16.4.2"},
		{"all hops trusted, several: the leftmost", "10.0.0.1:5000", []string{"172.16.9.9, 172.16.4.2"}, proxies, "172.16.9.9"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, resolveClientIP(c.remote, c.xff, c.proxies))
		})
	}
}
