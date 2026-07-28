package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/auth"
)

// loginFrom issues a login POST from a specific client IP (RemoteAddr), which the
// rate limiter keys on. Returns the status code.
func (h *harness) loginFrom(path, host, ip string, body any) int {
	h.t.Helper()
	b, err := json.Marshal(body)
	require.NoError(h.t, err)
	req, err := http.NewRequest(http.MethodPost, "http://"+host+path, bytes.NewReader(b))
	require.NoError(h.t, err)
	req.Host = host
	req.RemoteAddr = ip + ":40000"
	req.Header.Set("Content-Type", "application/json")
	rec := &responseRecorder{header: http.Header{}, body: &bytes.Buffer{}}
	h.h.ServeHTTP(rec, req)
	return rec.result().StatusCode
}

// TestLoginRateLimit: failed logins trip a 429 after the configured threshold, and
// the limit is enforced per-IP and per-username independently, on both surfaces.
func TestLoginRateLimit(t *testing.T) {
	// Threshold 3, a long window so it never lapses during the test.
	h := newHarnessLimited(t, true, auth.NewInMemoryLimiter(3, time.Hour, nil))
	h.seedCredentialedUser("carol", "correct-pw")
	h.seedAdmin("root", "admin-pw")

	badOwner := map[string]any{"username": "carol", "password": "wrong"}
	loginPath := "/api/v1/auth/login"

	// 3 failed owner logins for carol from IP .1 are 401; the 4th is 429.
	for i := 0; i < 3; i++ {
		assert.Equal(t, http.StatusUnauthorized, h.loginFrom(loginPath, h.ownerHost(), "10.0.0.1", badOwner), "attempt %d", i)
	}
	assert.Equal(t, http.StatusTooManyRequests, h.loginFrom(loginPath, h.ownerHost(), "10.0.0.1", badOwner))

	// Per-IP: a different username from the SAME IP is already blocked.
	assert.Equal(t, http.StatusTooManyRequests,
		h.loginFrom(loginPath, h.ownerHost(), "10.0.0.1", map[string]any{"username": "dave", "password": "x"}))

	// Per-username: carol from a DIFFERENT IP is blocked too (her identity tripped).
	assert.Equal(t, http.StatusTooManyRequests, h.loginFrom(loginPath, h.ownerHost(), "10.0.0.9", badOwner))

	// An unrelated username from the fresh IP still works (not globally blocked).
	assert.Equal(t, http.StatusUnauthorized,
		h.loginFrom(loginPath, h.ownerHost(), "10.0.0.9", map[string]any{"username": "erin", "password": "x"}))
}

// TestLoginRateLimitNotFoundCounts: attempts against a nonexistent account also
// count toward the limit — exercising the constant-time dummy-verify branch — so
// enumeration-by-flooding is throttled just like real accounts.
func TestLoginRateLimitNotFoundCounts(t *testing.T) {
	h := newHarnessLimited(t, false, auth.NewInMemoryLimiter(3, time.Hour, nil))
	ghost := map[string]any{"username": "ghost", "password": "x"}
	for i := 0; i < 3; i++ {
		assert.Equal(t, http.StatusUnauthorized, h.loginFrom("/api/v1/auth/login", h.ownerHost(), "10.0.0.5", ghost))
	}
	assert.Equal(t, http.StatusTooManyRequests, h.loginFrom("/api/v1/auth/login", h.ownerHost(), "10.0.0.5", ghost))
}

// TestAdminLoginRateLimit: the admin surface is rate-limited on the same terms.
func TestAdminLoginRateLimit(t *testing.T) {
	h := newHarnessLimited(t, true, auth.NewInMemoryLimiter(3, time.Hour, nil))
	h.seedAdmin("root", "admin-pw")
	bad := map[string]any{"username": "root", "password": "wrong"}
	for i := 0; i < 3; i++ {
		assert.Equal(t, http.StatusUnauthorized, h.loginFrom("/admin/auth/login", anyHost, "10.0.0.7", bad))
	}
	assert.Equal(t, http.StatusTooManyRequests, h.loginFrom("/admin/auth/login", anyHost, "10.0.0.7", bad))
}

// TestLoginRateLimitResetOnSuccess: a correct login clears the counter.
func TestLoginRateLimitResetOnSuccess(t *testing.T) {
	h := newHarnessLimited(t, false, auth.NewInMemoryLimiter(3, time.Hour, nil))
	h.seedCredentialedUser("carol", "correct-pw")
	host, ip := h.ownerHost(), "10.0.0.3"

	// Two failures, then a success resets carol + the IP.
	for i := 0; i < 2; i++ {
		assert.Equal(t, http.StatusUnauthorized, h.loginFrom("/api/v1/auth/login", host, ip,
			map[string]any{"username": "carol", "password": "wrong"}))
	}
	assert.Equal(t, http.StatusOK, h.loginFrom("/api/v1/auth/login", host, ip,
		map[string]any{"username": "carol", "password": "correct-pw"}))

	// The counter is cleared, so a fresh run of failures isn't immediately blocked.
	assert.Equal(t, http.StatusUnauthorized, h.loginFrom("/api/v1/auth/login", host, ip,
		map[string]any{"username": "carol", "password": "wrong"}))
}
