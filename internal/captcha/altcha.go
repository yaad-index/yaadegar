package captcha

import (
	"context"
	"fmt"
	"time"

	altchalib "github.com/altcha-org/altcha-lib-go"
)

// ProviderAltcha is the self-hosted proof-of-work provider (ADR-0013 cut 2). Unlike
// the managed token providers it makes no outbound call and needs no site key: the
// server issues an HMAC-signed challenge, the browser solves the proof-of-work, and
// Verify re-derives the solution and checks the signature locally with the shared
// secret as the HMAC key — zero third parties.
const ProviderAltcha = "altcha"

// altchaChallengeTTL bounds how long an issued challenge stays solvable. It also caps
// the window in which a solved payload could be replayed (cut 2 verifies signature +
// solution + expiry; a used-nonce store to reject in-window replay is a follow-up).
// Generous enough for a human to complete the sub-second solve and submit the reserve.
const altchaChallengeTTL = 5 * time.Minute

// altchaMaxNumber is the proof-of-work search space: the secret number is drawn from
// [0, altchaMaxNumber]. Sized so the browser solve stays well under a second while
// still costing a bot real work on every reserve attempt.
const altchaMaxNumber = 50_000

// Challenge is a server-issued, HMAC-signed proof-of-work challenge for the browser
// widget. Its fields mirror the Altcha challenge shape; the api layer maps it to the
// wire type, so this package needn't know the generated schema and the api package
// needn't know the Altcha library.
type Challenge struct {
	Algorithm string
	Challenge string
	MaxNumber int64
	Salt      string
	Signature string
}

// Challenger is implemented by verifiers whose scheme requires a server-issued
// challenge before the client can produce a token — the Altcha proof-of-work flow.
// The managed token providers do not implement it: their challenge is issued by the
// vendor SDK in the browser, so the challenge endpoint reports 404 for them.
type Challenger interface {
	// Challenge returns a freshly signed challenge for the widget to solve.
	Challenge(ctx context.Context) (Challenge, error)
}

// altchaVerifier implements both Verifier and Challenger for the Altcha proof-of-work
// scheme. It holds only the HMAC key (the instance secret); everything else is
// derived per challenge, and verification is fully local.
type altchaVerifier struct {
	hmacKey string
	now     func() time.Time // injectable for tests; defaults to time.Now
}

// Challenge issues a new signed challenge with a bounded expiry. The library embeds
// the expiry in the salt and signs the whole challenge, so Verify can enforce it
// without any server-side challenge store.
func (a *altchaVerifier) Challenge(_ context.Context) (Challenge, error) {
	expires := a.now().Add(altchaChallengeTTL)
	c, err := altchalib.CreateChallenge(altchalib.ChallengeOptions{
		HMACKey:   a.hmacKey,
		MaxNumber: altchaMaxNumber,
		Expires:   &expires,
	})
	if err != nil {
		return Challenge{}, fmt.Errorf("captcha: create altcha challenge: %w", err)
	}
	return Challenge{
		Algorithm: c.Algorithm,
		Challenge: c.Challenge,
		MaxNumber: c.MaxNumber,
		Salt:      c.Salt,
		Signature: c.Signature,
	}, nil
}

// Verify checks the base64 solution payload the widget submits: it re-derives the
// challenge from the payload's salt+number, confirms it matches, checks the HMAC
// signature against our key (constant-time), and rejects an expired challenge — all
// locally, with no outbound call. remoteIP is unused: Altcha is self-contained.
func (a *altchaVerifier) Verify(_ context.Context, token, _ string) error {
	ok, err := altchalib.VerifySolutionSafe(token, a.hmacKey, true)
	if err != nil {
		return fmt.Errorf("captcha: altcha verify: %w", err)
	}
	if !ok {
		return fmt.Errorf("captcha: altcha solution rejected")
	}
	return nil
}
