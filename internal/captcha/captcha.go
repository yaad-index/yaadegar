// Package captcha defines the human-verification seam for the low-trust public
// surfaces (self-registration, ADR-0012; low-trust reserve, ADR-0013). It is a
// pluggable extension point: a nil-default NoopVerifier so a disabled instance is
// unchanged, and managed-provider implementations (Turnstile, hCaptcha, reCAPTCHA)
// that slot in behind the same interface when the operator configures one.
package captcha

import "context"

// Verifier checks a client-submitted captcha token against the challenge provider,
// server-side. A nil error means the token passed (human); any error means refuse
// the request — a rejected challenge and a provider outage are the same "refuse"
// outcome to every caller, so the result is a single error rather than a separate
// pass/fail bool (ADR-0013 §1). remoteIP is the caller's IP, which some providers
// bind the challenge to; a verifier that does not use it ignores it.
type Verifier interface {
	Verify(ctx context.Context, token, remoteIP string) error
}

// NoopVerifier is the disabled default: it accepts every token so the low-trust
// paths work with no provider configured. It is the wiring for the seam; a real
// verifier (see New) replaces it when the operator turns captcha on.
type NoopVerifier struct{}

// Verify always accepts.
func (NoopVerifier) Verify(_ context.Context, _, _ string) error { return nil }
