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

// resetObs is the observable outcome of one password-reset request — the only two
// things a caller can see. The enumeration-safety guarantee is precisely that this
// pair is independent of whether the identifier resolves to an account.
type resetObs struct {
	status int
	body   string
}

// resetOne POSTs a single password-reset request from a given client IP (the limiter
// keys on RemoteAddr) and returns the observable outcome.
func resetOne(t *testing.T, h *harness, ip, identifier string) resetObs {
	t.Helper()
	b, err := json.Marshal(map[string]any{"identifier": identifier})
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPost,
		"http://"+h.ownerHost()+"/api/v1/auth/password-reset/request", bytes.NewReader(b))
	require.NoError(t, err)
	req.Host = h.ownerHost()
	req.RemoteAddr = ip + ":40000"
	req.Header.Set("Content-Type", "application/json")
	rec := &responseRecorder{header: http.Header{}, body: &bytes.Buffer{}}
	h.h.ServeHTTP(rec, req)
	return resetObs{status: rec.result().StatusCode, body: rec.body.String()}
}

// resetSeq runs n requests for identifier from ip and returns the outcome sequence.
func resetSeq(t *testing.T, h *harness, ip, identifier string, n int) []resetObs {
	t.Helper()
	seq := make([]resetObs, 0, n)
	for i := 0; i < n; i++ {
		seq = append(seq, resetOne(t, h, ip, identifier))
	}
	return seq
}

// TestPasswordResetRateLimit: reset requests trip a 429 after the configured
// threshold, enforced per-IP and per-identifier independently — the same keying as
// login, so a flood from one IP or against one identifier is throttled.
func TestPasswordResetRateLimit(t *testing.T) {
	// Threshold 3, a long window so it never lapses during the test.
	h := newHarnessLimited(t, auth.NewInMemoryLimiter(3, time.Hour, nil))
	h.seedOwnerWithEmail("erin", "erin@example.com", "first-password")

	// 3 requests for erin from IP .1 are 202; the 4th is 429.
	for i := 0; i < 3; i++ {
		assert.Equal(t, http.StatusAccepted, resetOne(t, h, "10.2.0.1", "erin@example.com").status, "attempt %d", i)
	}
	assert.Equal(t, http.StatusTooManyRequests, resetOne(t, h, "10.2.0.1", "erin@example.com").status)

	// Per-IP: a different identifier from the SAME IP is already blocked (the IP
	// bucket tripped), whether or not that identifier exists.
	assert.Equal(t, http.StatusTooManyRequests, resetOne(t, h, "10.2.0.1", "ghost").status)

	// Per-identifier: erin from a DIFFERENT IP is blocked too (her identity tripped).
	assert.Equal(t, http.StatusTooManyRequests, resetOne(t, h, "10.2.0.9", "erin@example.com").status)

	// An unrelated identifier from the fresh IP still works (not globally blocked).
	assert.Equal(t, http.StatusAccepted, resetOne(t, h, "10.2.0.9", "frank@example.com").status)
}

// TestPasswordResetRateLimitEnumerationSafe is the load-bearing test for issue #289:
// the throttle must not become the existence oracle the constant 202 refuses to be.
//
// A known identifier and an unknown one are each driven past the limit from their own
// IPs (threshold 3). The observable outcome — status AND body together — must be
// byte-identical at every step: under the limit (202, empty body) and over it (429,
// the generic problem body), and the limit must trip at the same request count for
// both. If existence changed the status, the body, or the trip point, the fix would
// have reintroduced exactly the leak the 202 was designed to prevent.
//
// (The timing dimension is covered structurally: the allow-check and the count run
// ahead of the account lookup and never branch on found-ness, so a found request does
// no throttle work an unknown one does not — see also
// TestPasswordResetRequestPersistsOffResponsePath for the send-path timing.)
func TestPasswordResetRateLimitEnumerationSafe(t *testing.T) {
	h := newHarnessLimited(t, auth.NewInMemoryLimiter(3, time.Hour, nil))
	h.seedOwnerWithEmail("erin", "erin@example.com", "first-password")

	known := resetSeq(t, h, "10.3.0.1", "erin@example.com", 4)
	unknown := resetSeq(t, h, "10.3.0.2", "ghost", 4)

	// The whole sequence — status and body at each step — is identical for a real and
	// a non-existent identifier.
	assert.Equal(t, unknown, known,
		"known and unknown identifiers must yield identical (status, body) sequences under and over the limit")

	// And concretely, so a regression names the shape it broke: 3×(202, empty) then 429.
	require.Len(t, known, 4)
	for i := 0; i < 3; i++ {
		assert.Equal(t, http.StatusAccepted, known[i].status, "under-limit request %d is 202", i)
		assert.Empty(t, known[i].body, "202 body is empty (request %d)", i)
	}
	assert.Equal(t, http.StatusTooManyRequests, known[3].status, "over-limit request is 429")
	assert.NotEmpty(t, known[3].body, "429 carries a generic problem body")
}

// TestPasswordResetRateLimitIPBucketBlindToExistence drives the same IP bucket with
// known and unknown identifiers interleaved: the IP-keyed limit counts every request
// regardless of which identifier it carried, so the 429 trips purely on request
// volume, never on whether the account exists. The response at the trip point is the
// same whether the tripping request named a real account or not.
func TestPasswordResetRateLimitIPBucketBlindToExistence(t *testing.T) {
	ip := "10.3.0.9"

	// Interleave real/unknown from one IP; the 4th request trips 429 no matter which
	// identifier carries it. Run it twice with the 4th request carrying, respectively,
	// a known and an unknown identifier, and assert the tripping response is identical.
	trip := func(fourth string) resetObs {
		h := newHarnessLimited(t, auth.NewInMemoryLimiter(3, time.Hour, nil))
		h.seedOwnerWithEmail("erin", "erin@example.com", "first-password")
		resetOne(t, h, ip, "erin@example.com")
		resetOne(t, h, ip, "ghost")
		resetOne(t, h, ip, "erin@example.com")
		return resetOne(t, h, ip, fourth)
	}
	tripKnown := trip("erin@example.com")
	tripUnknown := trip("ghost")
	assert.Equal(t, http.StatusTooManyRequests, tripKnown.status)
	assert.Equal(t, tripUnknown, tripKnown,
		"the over-limit response must not depend on whether the tripping identifier exists")
}
