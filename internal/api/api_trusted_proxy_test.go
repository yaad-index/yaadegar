package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/api"
	"github.com/yaad-index/yaadegar/internal/auth"
	"github.com/yaad-index/yaadegar/internal/clock"
)

// loginVia posts a failing login through a given peer, optionally carrying an
// X-Forwarded-For value, and returns the status. The username is derived from
// the forwarded value, because the login limiter also keys on the username: two
// clients sharing one would trip each other through that key, not the address.
func (h *harness) loginVia(peer, xff string) int {
	h.t.Helper()
	b, err := json.Marshal(map[string]string{"username": "nobody-" + xff, "password": "wrong"})
	require.NoError(h.t, err)
	req, err := http.NewRequest(http.MethodPost, "http://"+h.ownerHost()+"/api/v1/auth/login", bytes.NewReader(b))
	require.NoError(h.t, err)
	req.Host = h.ownerHost()
	req.RemoteAddr = peer + ":40000"
	req.Header.Set("Content-Type", "application/json")
	if xff != "" {
		req.Header.Set("X-Forwarded-For", xff)
	}
	rec := &responseRecorder{header: http.Header{}, body: &bytes.Buffer{}}
	h.h.ServeHTTP(rec, req)
	return rec.result().StatusCode
}

func newHarnessProxied(t *testing.T, limiter auth.Limiter, proxies []string) *harness {
	t.Helper()
	parsed, err := api.ParseTrustedProxies(proxies)
	require.NoError(t, err)
	return newHarnessFull(t, limiter, false, "", captchaConfig{}, "", 0, nil, 0, parsed)
}

// Behind a trusted proxy, one client's failures lock out that client only.
func TestLoginLimitKeysOnForwardedClientBehindTrustedProxy(t *testing.T) {
	lim := auth.NewInMemoryLimiter(2, time.Hour, clock.NewFake(time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)))
	h := newHarnessProxied(t, lim, []string{"10.0.0.1"})

	assert.Equal(t, http.StatusUnauthorized, h.loginVia("10.0.0.1", "203.0.113.5"))
	assert.Equal(t, http.StatusUnauthorized, h.loginVia("10.0.0.1", "203.0.113.5"))
	assert.Equal(t, http.StatusTooManyRequests, h.loginVia("10.0.0.1", "203.0.113.5"), "client A is limited")
	assert.Equal(t, http.StatusUnauthorized, h.loginVia("10.0.0.1", "203.0.113.9"), "client B behind the same proxy is not")
}

// Without trusted proxies the header is ignored: every client behind the proxy
// shares its address, which is the behaviour this change exists to fix for
// configured deployments.
func TestLoginLimitIgnoresForwardedForWithoutTrustedProxies(t *testing.T) {
	lim := auth.NewInMemoryLimiter(2, time.Hour, clock.NewFake(time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)))
	h := newHarnessProxied(t, lim, nil)

	h.loginVia("10.0.0.1", "203.0.113.5")
	h.loginVia("10.0.0.1", "203.0.113.5")
	assert.Equal(t, http.StatusTooManyRequests, h.loginVia("10.0.0.1", "203.0.113.9"), "all clients share the peer address")
}

// A peer that is not a trusted proxy cannot escape its limit by sending a new
// X-Forwarded-For value on every attempt.
func TestLoginLimitCannotBeEvadedWithSpoofedForwardedFor(t *testing.T) {
	lim := auth.NewInMemoryLimiter(2, time.Hour, clock.NewFake(time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)))
	h := newHarnessProxied(t, lim, []string{"10.0.0.1"})

	h.loginVia("198.51.100.7", "203.0.113.1")
	h.loginVia("198.51.100.7", "203.0.113.2")
	assert.Equal(t, http.StatusTooManyRequests, h.loginVia("198.51.100.7", "203.0.113.3"))
}
