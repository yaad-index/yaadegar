package captcha

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	altchalib "github.com/altcha-org/altcha-lib-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// solve runs the proof-of-work over an issued challenge and returns the base64 JSON
// solution payload the widget would submit as captcha_token. numberOverride, when
// non-nil, replaces the solved number to model a tampered submission.
func solve(t *testing.T, ch Challenge, numberOverride *int) string {
	t.Helper()
	sol, err := altchalib.SolveChallenge(ch.Challenge, ch.Salt, altchalib.Algorithm(ch.Algorithm), int(ch.MaxNumber), 0, nil)
	require.NoError(t, err)
	require.NotNil(t, sol, "challenge should be solvable within maxNumber")
	number := sol.Number
	if numberOverride != nil {
		number = *numberOverride
	}
	payload, err := json.Marshal(map[string]any{
		"algorithm": ch.Algorithm,
		"challenge": ch.Challenge,
		"number":    number,
		"salt":      ch.Salt,
		"signature": ch.Signature,
	})
	require.NoError(t, err)
	return base64.StdEncoding.EncodeToString(payload)
}

func newAltcha(t *testing.T, key string, now func() time.Time) *altchaVerifier {
	t.Helper()
	v, err := New(ProviderAltcha, key)
	require.NoError(t, err)
	av, ok := v.(*altchaVerifier)
	require.True(t, ok, "altcha provider must build an *altchaVerifier")
	if now != nil {
		av.now = now
	}
	return av
}

func TestNewAltcha(t *testing.T) {
	v, err := New(ProviderAltcha, "hmac-key")
	require.NoError(t, err)
	require.NotNil(t, v) // New returns a Verifier by construction
	_, isChallenger := v.(Challenger)
	assert.True(t, isChallenger, "altcha also builds a Challenger (server-issued challenge)")

	// A missing secret fails closed: altcha needs the HMAC key.
	_, err = New(ProviderAltcha, "")
	require.Error(t, err)
}

func TestAltchaChallengeShape(t *testing.T) {
	v := newAltcha(t, "hmac-key", nil)
	ch, err := v.Challenge(context.Background())
	require.NoError(t, err)
	assert.Equal(t, string(altchalib.SHA256), ch.Algorithm)
	assert.Equal(t, int64(altchaMaxNumber), ch.MaxNumber)
	assert.NotEmpty(t, ch.Challenge)
	assert.NotEmpty(t, ch.Salt)
	assert.NotEmpty(t, ch.Signature)
}

func TestAltchaVerifyRoundtrip(t *testing.T) {
	v := newAltcha(t, "hmac-key", nil)
	ch, err := v.Challenge(context.Background())
	require.NoError(t, err)

	token := solve(t, ch, nil)
	require.NoError(t, v.Verify(context.Background(), token, ""), "a correctly solved challenge passes")
}

func TestAltchaVerifyRejectsTamperedNumber(t *testing.T) {
	v := newAltcha(t, "hmac-key", nil)
	ch, err := v.Challenge(context.Background())
	require.NoError(t, err)

	// A number that does not hash to the challenge must be refused (the whole point of
	// the proof-of-work): the re-derived challenge won't match the signed one.
	wrong := int(ch.MaxNumber) + 1
	token := solve(t, ch, &wrong)
	assert.Error(t, v.Verify(context.Background(), token, ""))
}

func TestAltchaVerifyRejectsForeignKey(t *testing.T) {
	// A challenge signed by a different instance secret must not verify here: the HMAC
	// signature is keyed by the secret, so a solution minted elsewhere is refused.
	foreign := newAltcha(t, "some-other-instance-key", nil)
	ch, err := foreign.Challenge(context.Background())
	require.NoError(t, err)
	token := solve(t, ch, nil)

	ours := newAltcha(t, "hmac-key", nil)
	assert.Error(t, ours.Verify(context.Background(), token, ""))
}

func TestAltchaVerifyRejectsExpired(t *testing.T) {
	// Issue a challenge whose embedded expiry is already in the past (the verifier's
	// clock is pinned far back), then verify against the real clock: the TTL is
	// enforced server-side, so an expired-but-correctly-solved challenge is refused.
	past := func() time.Time { return time.Unix(1_000_000, 0) }
	v := newAltcha(t, "hmac-key", past)
	ch, err := v.Challenge(context.Background())
	require.NoError(t, err)
	token := solve(t, ch, nil)

	assert.Error(t, v.Verify(context.Background(), token, ""), "an expired challenge is refused")
}

func TestAltchaVerifyRejectsGarbageToken(t *testing.T) {
	v := newAltcha(t, "hmac-key", nil)
	assert.Error(t, v.Verify(context.Background(), "not-base64-json", ""))
}
