package api_test

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"testing"

	altchalib "github.com/altcha-org/altcha-lib-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/api/gen"
	"github.com/yaad-index/yaadegar/internal/captcha"
)

// TestCaptchaChallengeAltcha exercises the full self-hosted proof-of-work loop
// server-side (ADR-0013 cut 2): the instance issues a signed challenge, the test
// solves it exactly as the browser widget would, and the solved payload verifies on
// the low-trust reserve path — no outbound call, no browser. It complements the
// real-browser widget test (which covers the client SDK render + solve).
func TestCaptchaChallengeAltcha(t *testing.T) {
	verifier, err := captcha.New(captcha.ProviderAltcha, "instance-hmac-key")
	require.NoError(t, err)
	h := newHarnessCaptcha(t, captchaConfig{verifier: verifier, provider: "altcha", siteKey: ""})

	// The instance hands out a signed challenge on the browser-reachable public path.
	resp, body := h.req(http.MethodGet, "/api/v1/public/captcha/challenge", h.ownerHost(), "", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	ch := decode[gen.AltchaChallenge](t, body)
	assert.NotEmpty(t, ch.Challenge)
	assert.NotEmpty(t, ch.Salt)
	assert.NotEmpty(t, ch.Signature)
	assert.Positive(t, ch.Maxnumber)

	// Solve the proof-of-work and package the solution the way the widget submits it.
	sol, err := altchalib.SolveChallenge(ch.Challenge, ch.Salt, altchalib.Algorithm(ch.Algorithm), int(ch.Maxnumber), 0, nil)
	require.NoError(t, err)
	require.NotNil(t, sol)
	payload, err := json.Marshal(map[string]any{
		"algorithm": ch.Algorithm,
		"challenge": ch.Challenge,
		"number":    sol.Number,
		"salt":      ch.Salt,
		"signature": ch.Signature,
	})
	require.NoError(t, err)
	token := base64.StdEncoding.EncodeToString(payload)

	// The solved token clears the low-trust reserve gate end-to-end.
	list := h.createList("Guest list") // full_guest (instance default)
	item := h.createItem(*list.Id, "Item", 1)
	resp, rbody := h.req(http.MethodPost, "/public/"+*list.ShareSlug+"/items/"+*item.Id+"/reservations",
		h.ownerHost(), "", map[string]any{"quantity": 1, "captcha_token": token})
	require.Equal(t, http.StatusCreated, resp.StatusCode, "body: %s", rbody)
	assert.NotNil(t, decode[gen.ReservationCreated](t, rbody).CapabilityToken)
}

// TestCaptchaChallengeAbsentWithoutAltcha: the challenge endpoint is Altcha-only. A
// managed token provider (its challenge is vendor-issued in the browser) and a
// disabled instance both have nothing to issue, so the endpoint reports 404.
func TestCaptchaChallengeAbsentWithoutAltcha(t *testing.T) {
	managed := newHarnessCaptcha(t, captchaConfig{verifier: &scriptedVerifier{}, provider: "turnstile", siteKey: "sk"})
	resp, _ := managed.req(http.MethodGet, "/api/v1/public/captcha/challenge", managed.ownerHost(), "", nil)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode, "managed provider issues no server-side challenge")

	off := newHarness(t) // captcha disabled (NoopVerifier)
	resp, _ = off.req(http.MethodGet, "/api/v1/public/captcha/challenge", off.ownerHost(), "", nil)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode, "disabled instance issues no challenge")
}

// TestCaptchaChallengeUnauthenticated guards the middleware carve-out: the challenge
// lives under /api/v1/public/ and must be reachable with no bearer token (the giver
// is anonymous), unlike the rest of the /api/v1 owner surface.
func TestCaptchaChallengeUnauthenticated(t *testing.T) {
	verifier, err := captcha.New(captcha.ProviderAltcha, "instance-hmac-key")
	require.NoError(t, err)
	h := newHarnessCaptcha(t, captchaConfig{verifier: verifier, provider: "altcha", siteKey: ""})

	// No token, no cookie — a bare anonymous GET.
	resp, _ := h.req(http.MethodGet, "/api/v1/public/captcha/challenge", h.ownerHost(), "", nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
