package api_test

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/storage"
)

// The rest of api_register_test.go posts captcha_token: "" throughout and passes,
// because with the default no-op verifier that IS the correct outcome — so the
// existing suite only ever exercises registration in the one configuration where a
// captcha defect cannot appear. These run the register path with a real verifier
// configured, which is the configuration #384 was reported from.

// TestRegisterCaptchaGate: with a verifier configured, an absent or empty token is
// refused BEFORE the provider is called, a rejected token is a 400, and an accepted
// token registers. The "before the provider" assertions are the point: they hold the
// endpoint to refusing an empty token itself, rather than resting on the verifier
// happening to reject "" — which is a property of whichever provider is configured
// and not a guarantee of the endpoint.
func TestRegisterCaptchaGate(t *testing.T) {
	v := &scriptedVerifier{}
	h := newHarnessRegistrationCaptcha(t, storage.RegistrationGiversOnly,
		captchaConfig{verifier: v, provider: "turnstile", siteKey: "site-abc"})

	register := func(body map[string]any) (*http.Response, []byte) {
		return h.req(http.MethodPost, "/api/v1/auth/register", h.ownerHost(), "", body)
	}
	const email = "newbie@example.com"

	// Absent token → 400, and the provider is never reached.
	resp, body := register(map[string]any{"email": email, "password": "long-enough-pass"})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode, "body: %s", body)
	assert.Empty(t, v.calls, "an absent token must not reach the provider")

	// Present-but-empty token → the same refusal, still without a provider call. This
	// is the shape the page actually posts when the widget has not resolved.
	resp, _ = register(map[string]any{"email": email, "password": "long-enough-pass", "captcha_token": ""})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Empty(t, v.calls, "an empty token must not reach the provider")

	// Present-but-rejected token → 400; the provider was called with that token.
	v.err = errors.New("challenge failed")
	resp, _ = register(map[string]any{"email": email, "password": "long-enough-pass", "captcha_token": "bad-token"})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.Len(t, v.calls, 1)
	assert.Equal(t, "bad-token", v.calls[0].token)

	// Nothing above created an account — the gate runs before any account work.
	_, err := h.userByEmail(email)
	assert.ErrorIs(t, err, storage.ErrNotFound, "a refused registration creates no account")

	// Accepted token → 202, the provider was consulted, and the pending account lands.
	v.err = nil
	resp, abody := register(map[string]any{"email": email, "password": "long-enough-pass", "captcha_token": "good-token"})
	require.Equal(t, http.StatusAccepted, resp.StatusCode, "body: %s", abody)
	require.Len(t, v.calls, 2)
	assert.Equal(t, "good-token", v.calls[1].token)

	require.Eventually(t, func() bool {
		_, err := h.userByEmail(email)
		return err == nil
	}, time.Second, 10*time.Millisecond, "the accepted registration creates the pending account")
	created, err := h.userByEmail(email)
	require.NoError(t, err)
	assert.Equal(t, storage.UserStatusPending, created.Status)
}

// TestRegisterCaptchaDisabledUnchanged: with no verifier configured, the register
// path is byte-for-byte what it was — an empty token still registers. This is the
// guard on the gate itself: it must not start refusing registrations on the
// (default) instances that never configured a provider.
func TestRegisterCaptchaDisabledUnchanged(t *testing.T) {
	h := newHarnessRegistration(t, storage.RegistrationGiversOnly) // captcha disabled

	resp, body := h.req(http.MethodPost, "/api/v1/auth/register", h.ownerHost(), "",
		map[string]any{"email": "newbie@example.com", "password": "long-enough-pass", "captcha_token": ""})
	require.Equal(t, http.StatusAccepted, resp.StatusCode, "body: %s", body)

	require.Eventually(t, func() bool {
		_, err := h.userByEmail("newbie@example.com")
		return err == nil
	}, time.Second, 10*time.Millisecond)
}
