// Package captcha defines the human-verification seam for the self-registration
// path (ADR-0012 cut 1a). It is a seam, not an implementation: cut 1a ships only the
// no-op verifier so the register flow is wired and testable end-to-end, and a real
// provider (e.g. a hosted challenge) slots in later behind the same interface
// without touching the handler.
package captcha

import "context"

// Verifier checks a client-submitted captcha token against the challenge provider.
// It returns whether the token is a valid human-solved challenge. remoteIP is the
// caller's IP, which some providers bind the challenge to; a verifier that does not
// use it simply ignores it.
type Verifier interface {
	Verify(ctx context.Context, token, remoteIP string) (bool, error)
}

// NoopVerifier is the disabled default: it accepts every token (returns true) so the
// register path works with no provider configured. It is the wiring for cut 1a; a
// real verifier replaces it when captcha is turned on.
type NoopVerifier struct{}

// Verify always accepts.
func (NoopVerifier) Verify(_ context.Context, _, _ string) (bool, error) { return true, nil }
