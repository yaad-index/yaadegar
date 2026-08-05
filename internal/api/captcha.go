package api

import (
	"context"

	"github.com/yaad-index/yaadegar/internal/api/gen"
	"github.com/yaad-index/yaadegar/internal/captcha"
)

// GetCaptchaChallenge issues a signed Altcha proof-of-work challenge for the giver
// widget (ADR-0013 cut 2). It is unauthenticated and instance-level. Only the Altcha
// provider issues a server-side challenge: it is the one verifier that implements
// captcha.Challenger. Every managed token provider (its challenge is vendor-issued in
// the browser) and a disabled instance (the no-op verifier) leaves the assertion
// failing, so the endpoint reports 404 — there is nothing to hand out.
func (s *Server) GetCaptchaChallenge(ctx context.Context, _ gen.GetCaptchaChallengeRequestObject) (gen.GetCaptchaChallengeResponseObject, error) {
	challenger, ok := s.captcha.(captcha.Challenger)
	if !ok {
		return gen.GetCaptchaChallenge404ApplicationProblemPlusJSONResponse(
			problemDetail(404, "no captcha challenge is available on this instance"),
		), nil
	}
	c, err := challenger.Challenge(ctx)
	if err != nil {
		return nil, err
	}
	return gen.GetCaptchaChallenge200JSONResponse(gen.AltchaChallenge{
		Algorithm: c.Algorithm,
		Challenge: c.Challenge,
		Maxnumber: c.MaxNumber,
		Salt:      c.Salt,
		Signature: c.Signature,
	}), nil
}
