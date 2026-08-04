package api_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaad-index/yaadegar/internal/api/gen"
)

// scriptedVerifier records every token/IP the reserve path asks it to verify and
// returns a scripted result, so a test can assert whether the gate reached the
// provider and drive the pass/refuse outcome (ADR-0013). Reserve is synchronous, so
// no locking is needed.
type scriptedVerifier struct {
	err   error
	calls []scriptedCall
}

type scriptedCall struct{ token, ip string }

func (v *scriptedVerifier) Verify(_ context.Context, token, remoteIP string) error {
	v.calls = append(v.calls, scriptedCall{token, remoteIP})
	return v.err
}

// TestReserveCaptchaGateFullGuest: on a full_guest list with a verifier configured,
// an absent token is refused BEFORE the provider is called, a rejected token is a
// 400, and an accepted token reserves — proving the low-trust gate runs and is
// fail-closed on the empty-token case.
func TestReserveCaptchaGateFullGuest(t *testing.T) {
	v := &scriptedVerifier{}
	h := newHarnessCaptcha(t, captchaConfig{verifier: v, provider: "turnstile", siteKey: "site-abc"})
	list := h.createList("Guest list") // full_guest (instance default)
	item := h.createItem(*list.Id, "Item", 3)
	slug, itemID := *list.ShareSlug, *item.Id

	reserve := func(body map[string]any) (*http.Response, []byte) {
		return h.req(http.MethodPost, "/public/"+slug+"/items/"+itemID+"/reservations", h.ownerHost(), "", body)
	}

	// Absent token → 400, and the provider is never called (never verify "").
	resp, body := reserve(map[string]any{"quantity": 1})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode, "body: %s", body)
	assert.Empty(t, v.calls, "an empty token must not reach the provider")

	// Present-but-rejected token → 400; the provider was called with that token.
	v.err = errors.New("challenge failed")
	resp, _ = reserve(map[string]any{"quantity": 1, "captcha_token": "bad-token"})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.Len(t, v.calls, 1)
	assert.Equal(t, "bad-token", v.calls[0].token)

	// Accepted token → 201 active with a capability token.
	v.err = nil
	resp, cbody := reserve(map[string]any{"quantity": 1, "captcha_token": "good-token"})
	require.Equal(t, http.StatusCreated, resp.StatusCode, "body: %s", cbody)
	created := decode[gen.ReservationCreated](t, cbody)
	assert.Equal(t, gen.ReservationCreatedStatusActive, created.Status)
	assert.NotNil(t, created.CapabilityToken)
	require.Len(t, v.calls, 2)
	assert.Equal(t, "good-token", v.calls[1].token)
}

// TestReserveCaptchaGateEmailConfirmed: the email_confirmed tier is also low-trust,
// so it is gated too — an absent token is refused before Verify (and before the
// email-required check), and a valid token lets the pending-confirmation reserve
// through.
func TestReserveCaptchaGateEmailConfirmed(t *testing.T) {
	v := &scriptedVerifier{}
	h := newHarnessCaptcha(t, captchaConfig{verifier: v, provider: "hcaptcha", siteKey: "sk"})
	_, lbody := h.req(http.MethodPost, "/api/v1/lists", h.ownerHost(), h.ownerToken(),
		gen.ListCreate{Title: "Confirm", ReserverTier: sptr("email_confirmed")})
	list := decode[gen.List](t, lbody)
	item := h.createItem(*list.Id, "Item", 1)
	slug, itemID := *list.ShareSlug, *item.Id

	reserve := func(body map[string]any) (*http.Response, []byte) {
		return h.req(http.MethodPost, "/public/"+slug+"/items/"+itemID+"/reservations", h.ownerHost(), "", body)
	}

	// Absent token → 400 before the provider (and before the email check).
	resp, _ := reserve(map[string]any{"giver_email": "giver@example.com"})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Empty(t, v.calls)

	// Valid token → 202 pending_confirmation, and the provider was consulted.
	resp, rbody := reserve(map[string]any{"giver_email": "giver@example.com", "captcha_token": "ok"})
	require.Equal(t, http.StatusAccepted, resp.StatusCode, "body: %s", rbody)
	assert.Equal(t, gen.ReservationCreatedStatusPendingConfirmation, decode[gen.ReservationCreated](t, rbody).Status)
	require.Len(t, v.calls, 1)
	assert.Equal(t, "ok", v.calls[0].token)
}

// TestReserveCaptchaDisabledNoGate: with no verifier configured, the low-trust
// reserve path is unchanged — a reserve with no token still succeeds, and the
// verify seam is never a factor.
func TestReserveCaptchaDisabledNoGate(t *testing.T) {
	h := newHarness(t) // captcha disabled (NoopVerifier)
	list := h.createList("Open list")
	item := h.createItem(*list.Id, "Item", 1)

	resp, created := h.reserve(*list.ShareSlug, *item.Id, 1) // no captcha_token
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	assert.NotNil(t, created.CapabilityToken)
}

// TestAuthMethodsCaptchaExposure: the public auth-methods surface carries the
// provider + public site key when captcha is enabled (so the giver page can render
// the widget), and omits them entirely when disabled.
func TestAuthMethodsCaptchaExposure(t *testing.T) {
	v := &scriptedVerifier{}
	on := newHarnessCaptcha(t, captchaConfig{verifier: v, provider: "turnstile", siteKey: "site-xyz"})
	_, body := on.req(http.MethodGet, "/api/v1/auth/methods", on.ownerHost(), "", nil)
	m := decode[gen.LoginMethods](t, body)
	require.NotNil(t, m.CaptchaProvider)
	assert.Equal(t, "turnstile", *m.CaptchaProvider)
	require.NotNil(t, m.CaptchaSiteKey)
	assert.Equal(t, "site-xyz", *m.CaptchaSiteKey)

	off := newHarness(t)
	_, body = off.req(http.MethodGet, "/api/v1/auth/methods", off.ownerHost(), "", nil)
	m = decode[gen.LoginMethods](t, body)
	assert.Nil(t, m.CaptchaProvider, "disabled instance must not surface a provider")
	assert.Nil(t, m.CaptchaSiteKey)
}
