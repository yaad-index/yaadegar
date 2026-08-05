package captcha

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Provider names accepted by New (and the YAADEGAR_CAPTCHA_PROVIDER config). None
// (or empty) means disabled; the others are the managed token-verification
// providers (ADR-0013 §1).
const (
	ProviderNone      = "none"
	ProviderTurnstile = "turnstile"
	ProviderHCaptcha  = "hcaptcha"
	ProviderRecaptcha = "recaptcha"
)

// VerifyTimeout bounds a single provider verify call (ADR-0013 §6). Fixed here so
// every implementor shares one value rather than each picking their own; a provider
// slower than this is treated as an outage and, being an error, fails closed.
const VerifyTimeout = 5 * time.Second

// siteVerifyEndpoints maps each managed provider to its server-side verify URL. All
// three share one shape: POST form-encoded secret+response, receive {"success": …}.
var siteVerifyEndpoints = map[string]string{
	ProviderTurnstile: "https://challenges.cloudflare.com/turnstile/v0/siteverify",
	ProviderHCaptcha:  "https://api.hcaptcha.com/siteverify",
	ProviderRecaptcha: "https://www.google.com/recaptcha/api/siteverify",
}

// New builds a Verifier for the named provider. "none" or "" returns (nil, nil) —
// the caller treats a nil Verifier as disabled and installs the NoopVerifier. A
// known provider with an empty secret, or an unknown provider, is an error so a
// misconfigured instance fails closed at startup rather than silently running open.
func New(provider, secret string) (Verifier, error) {
	switch provider {
	case "", ProviderNone:
		return nil, nil
	case ProviderTurnstile, ProviderHCaptcha, ProviderRecaptcha:
		if secret == "" {
			return nil, fmt.Errorf("captcha: provider %q requires a secret", provider)
		}
		return &managedVerifier{
			endpoint: siteVerifyEndpoints[provider],
			secret:   secret,
			client:   &http.Client{Timeout: VerifyTimeout},
		}, nil
	case ProviderAltcha:
		// Self-hosted proof-of-work (ADR-0013 cut 2): the secret is the HMAC key that
		// signs challenges and authenticates solutions. No site key, no outbound call.
		if secret == "" {
			return nil, fmt.Errorf("captcha: provider %q requires a secret (used as the HMAC key)", provider)
		}
		return &altchaVerifier{hmacKey: secret, now: time.Now}, nil
	default:
		return nil, fmt.Errorf("captcha: unknown provider %q (want %s|%s|%s|%s|%s)",
			provider, ProviderTurnstile, ProviderHCaptcha, ProviderRecaptcha, ProviderAltcha, ProviderNone)
	}
}

// managedVerifier implements Verifier for the token-verification providers
// (Turnstile, hCaptcha, reCAPTCHA), which share one protocol: POST the secret and
// the client-supplied response token to a siteverify endpoint and read back a JSON
// body with a boolean "success". Any transport error, non-200, undecodable body, or
// success=false is a refusal (fail-closed).
type managedVerifier struct {
	endpoint string
	secret   string
	client   *http.Client
}

func (m *managedVerifier) Verify(ctx context.Context, token, remoteIP string) error {
	form := url.Values{"secret": {m.secret}, "response": {token}}
	if remoteIP != "" {
		form.Set("remoteip", remoteIP)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("captcha: build verify request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := m.client.Do(req)
	if err != nil {
		return fmt.Errorf("captcha: verify request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("captcha: provider returned status %d", resp.StatusCode)
	}
	var out struct {
		Success    bool     `json:"success"`
		ErrorCodes []string `json:"error-codes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return fmt.Errorf("captcha: decode verify response: %w", err)
	}
	if !out.Success {
		return fmt.Errorf("captcha: challenge rejected: %v", out.ErrorCodes)
	}
	return nil
}
