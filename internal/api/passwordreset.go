package api

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/yaad-index/yaadegar/internal/api/gen"
	"github.com/yaad-index/yaadegar/internal/auth"
	"github.com/yaad-index/yaadegar/internal/email"
	"github.com/yaad-index/yaadegar/internal/storage"
	"github.com/yaad-index/yaadegar/internal/token"
)

// passwordResetTTL is how long an emailed reset link stays valid (ADR-0011 cut 3):
// short, single-use, so a leaked-then-expired link is inert.
const passwordResetTTL = time.Hour

// setPasswordInviteTTL is how long an admin invite / first-password link stays valid
// (ADR-0012 cut 1b). It is longer than passwordResetTTL because an invited person
// acts on their own schedule, not right after a self-initiated request. The link is
// still single-use and hashed, and a fresh one is always self-servable from the
// forgot-password path (resettable now covers no-password accounts), so a lapsed
// invite is a recoverable inconvenience, not a lockout.
const setPasswordInviteTTL = 72 * time.Hour

// RequestPasswordReset starts the forgot-password flow (ADR-0011 cut 3). It is
// enumeration-safe: under the rate limit the response is a 202 with no body, whether
// or not the identifier resolves to an account, and over the limit it is a 429 —
// applied identically regardless of existence (see resetKeys). Four things keep
// existence from leaking:
//   - the throttle keys, allow-check, and count all run before the lookup and never
//     branch on found-ness, so the limiter state is not a tell (#289);
//   - the token mint (crypto/rand + hash) runs on every request, found or not, so
//     that constant work is not a tell;
//   - the token persist AND the email send both run off the response path (one
//     goroutine, ordered persist→send), so a found identifier does no extra
//     synchronous DB work than an unknown one (#159) — the request path does the
//     same work either way: throttle, mint, look up, maybe enqueue, return 202;
//   - failures inside the goroutine are logged, never surfaced.
func (s *Server) RequestPasswordReset(ctx context.Context, req gen.RequestPasswordResetRequestObject) (gen.RequestPasswordResetResponseObject, error) {
	ts, tenant, ok := s.tenantStore(ctx)
	if !ok {
		return nil, errMissingContext
	}
	if req.Body == nil || req.Body.Identifier == "" {
		return gen.RequestPasswordReset400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: badRequest("identifier is required"),
		}, nil
	}

	// Throttle before any account work — and, critically, identically for every
	// request regardless of whether the identifier resolves. The allow-check and the
	// count both run here, ahead of the lookup, and neither branches on found-ness, so
	// the limiter state stays a pure function of (IP, identifier, timing). If it
	// varied with existence it would become exactly the oracle the constant 202 exists
	// to deny. Over the limit every caller gets the same 429; the send path is
	// unreachable, so it too reveals nothing.
	ipKey, idKey := s.resetKeys(ctx, tenant.ID, req.Body.Identifier)
	if !s.resetAllowed(ipKey, idKey) {
		return gen.RequestPasswordReset429ApplicationProblemPlusJSONResponse{
			TooManyRequestsApplicationProblemPlusJSONResponse: tooManyRequests(),
		}, nil
	}
	s.resetRecord(ipKey, idKey)

	// Mint the token up front so the same crypto work is spent on every request.
	raw, hash, err := token.New()
	if err != nil {
		return nil, err
	}

	if user, found := s.lookupResettable(ctx, ts, req.Body.Identifier); found {
		expires := s.clock.Now().Add(passwordResetTTL)
		s.sendResetEmailAsync(tenant, user.ID, user.Email, raw, hash, expires)
	}

	// Always the same: 202, no body, no hint about what happened.
	return gen.RequestPasswordReset202Response{}, nil
}

// resetKeys returns the (ip, identity) rate-limit keys for a password-reset request,
// mirroring login's keying (client IP plus tenant-scoped identity). They ride the
// same limiter mechanism as login but under a distinct "pwreset:" namespace, so the
// two surfaces throttle independently: a reset flood never consumes a login bucket,
// and — the reason a shared key would be a bug, not just untidy — a successful login
// (which clears its keys) can never refill a reset window, nor a reset a login one.
// The identity key uses the SUBMITTED identifier verbatim: the same key is formed and
// counted whether or not it resolves, which is what keeps the throttle existence-blind.
func (s *Server) resetKeys(ctx context.Context, tenantID, identifier string) (ipKey, idKey string) {
	return "pwreset:ip:" + clientIPFromContext(ctx), "pwreset:id:" + tenantID + ":" + identifier
}

// resetAllowed reports whether both the IP and the identifier are under their limits.
func (s *Server) resetAllowed(ipKey, idKey string) bool {
	return s.loginLimiter.Allow(ipKey) && s.loginLimiter.Allow(idKey)
}

// resetRecord counts one reset request against both keys. Unlike login there is no
// success/failure distinction to record: every request counts (the limiter's
// RecordFailure is used purely as "this attempt consumed one unit of the window"),
// and success is never recorded — clearing on a resolved identifier would leak
// existence. So the pwreset keys behave as a plain per-window request counter.
func (s *Server) resetRecord(ipKey, idKey string) {
	s.loginLimiter.RecordFailure(ipKey)
	s.loginLimiter.RecordFailure(idKey)
}

// lookupResettable resolves an identifier (username or email) to an account that may
// receive a reset / establish-password link — a deliverable email on a non-banned
// account (see resettable). Any miss returns found=false so the caller stays silent.
func (s *Server) lookupResettable(ctx context.Context, ts storage.TenantStore, identifier string) (storage.User, bool) {
	if u, err := ts.Users().ByUsername(ctx, identifier); err == nil {
		return u, resettable(u)
	}
	if u, err := ts.Users().ByEmail(ctx, identifier); err == nil {
		return u, resettable(u)
	}
	return storage.User{}, false
}

// resettable reports whether an account may receive a reset / establish-password
// link. Eligibility is a deliverable email on a non-banned account — deliberately
// NOT conditioned on an existing password (ADR-0012 Decision 2 / cut 1b): a
// no-password account (admin-invited or OAuth-created) must be able to establish a
// FIRST password through this same flow, so the request path serves both "set first"
// and "reset". The confirm path sets the hash and bumps credential_version either way.
func resettable(u storage.User) bool {
	return u.Email != "" && !u.Banned
}

// sendResetEmailAsync persists the reset token and emails the link, both off the
// request path (#159) so a found identifier costs no more synchronous work than an
// unknown one. The order is persist→send: the emailed token must exist before the
// link goes out, so a persist failure aborts the send (a link whose token was never
// stored would be dead on arrival). Failures are logged, never surfaced (that would
// leak existence). The tenant store is re-derived here rather than captured from the
// request, so it is not tied to the request's lifecycle.
func (s *Server) sendResetEmailAsync(tenant storage.Tenant, userID, to, rawToken, tokenHash string, expires time.Time) {
	link := s.resetLink(tenant, rawToken)
	go func() {
		ctx := context.Background()
		ts := s.store.ForTenant(tenant)
		if _, err := ts.PasswordResetTokens().Create(ctx, storage.PasswordResetToken{
			UserID: userID, TokenHash: tokenHash, ExpiresAt: expires,
		}); err != nil {
			s.logger.ErrorContext(ctx, "password reset: persist token failed", "error", err)
			return
		}
		if err := s.email.Send(ctx, email.Message{
			To:      to,
			Subject: "Reset your password",
			Body: "Reset your password: " + link +
				"\n\nThis link can be used once and expires soon. If you didn't request it, you can ignore this email.",
		}); err != nil {
			s.logger.ErrorContext(ctx, "password reset: send email failed", "error", err)
		}
	}()
}

// sendInviteEmailAsync emails a first-password / set-password invite off the request
// path (ADR-0012 cut 1b). It reuses the reset link + confirm machinery — the same
// /reset landing sets the password and logs the person in — with invite-appropriate
// copy. Failures are logged, never fatal to account creation.
func (s *Server) sendInviteEmailAsync(tenant storage.Tenant, to, rawToken string) {
	link := s.resetLink(tenant, rawToken)
	go func() {
		ctx := context.Background()
		if err := s.email.Send(ctx, email.Message{
			To:      to,
			Subject: "Set your password",
			Body: "An account was created for you. Set your password to finish setting up and sign in: " + link +
				"\n\nThis link can be used once and expires in a few days. If it expires, use the \"forgot password\" link on the sign-in page to get a new one.",
		}); err != nil {
			s.logger.ErrorContext(ctx, "invite: send email failed", "error", err)
		}
	}()
}

// resetLink builds the tenant-correct reset URL. When a base domain is configured
// the link targets the owner's own subdomain host (so the confirm POST resolves the
// right tenant by Host, mirroring the owner login URL); otherwise it falls back to
// the configured public link base. The raw token is base64url — already URL-safe.
func (s *Server) resetLink(tenant storage.Tenant, rawToken string) string {
	base := s.publicLinkBase
	if s.baseDomain != "" {
		base = "https://" + tenant.Subdomain + "." + s.baseDomain
	}
	return base + "/reset?token=" + rawToken
}

// ConfirmPasswordReset completes the forgot-password flow (ADR-0011 cut 3): it
// validates the single-use token, routes the new password through the shared
// HashNewPassword funnel (which bumps credential_version and so drops every existing
// session), claims the token, and auto-issues a session so the caller lands logged
// in. An absent, expired, or already-used token is one generic 400.
func (s *Server) ConfirmPasswordReset(ctx context.Context, req gen.ConfirmPasswordResetRequestObject) (gen.ConfirmPasswordResetResponseObject, error) {
	ts, tenant, ok := s.tenantStore(ctx)
	if !ok {
		return nil, errMissingContext
	}
	if req.Body == nil || req.Body.Token == "" || req.Body.NewPassword == "" {
		return gen.ConfirmPasswordReset400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: badRequest("token and new_password are required"),
		}, nil
	}

	prt, err := ts.PasswordResetTokens().ByHash(ctx, token.Hash(req.Body.Token))
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return invalidResetToken(), nil
		}
		return nil, err
	}
	now := s.clock.Now()
	if prt.UsedAt != nil || !now.Before(prt.ExpiresAt) {
		return invalidResetToken(), nil
	}

	// Validate the new password BEFORE consuming the token, so a policy failure lets
	// the user retry with the same link rather than burning it.
	hash, err := auth.HashNewPassword(req.Body.NewPassword)
	if err != nil {
		if errors.Is(err, auth.ErrPasswordTooShort) {
			return gen.ConfirmPasswordReset400ApplicationProblemPlusJSONResponse{
				BadRequestApplicationProblemPlusJSONResponse: badRequest(
					fmt.Sprintf("the new password must be at least %d characters", auth.MinPasswordLen)),
			}, nil
		}
		return nil, err
	}

	// Apply the confirm mutation set atomically (#166): claim the token, set the new
	// password (bumping credential_version → drops every session), and activate a
	// still-pending account — all in one transaction, so the account never lands in a
	// partial state. Establishing a password through an emailed link proves email
	// ownership, which is why it also completes activation (ADR-0012 cut 1b); without
	// it a pending account would auto-login once then be unable to log in again (the
	// login gate rejects pending). claimed=false = a concurrent confirm already won.
	claimed, err := ts.PasswordResetTokens().ConfirmReset(ctx, prt.ID, prt.UserID, hash, now)
	if err != nil {
		return nil, err
	}
	if !claimed {
		return invalidResetToken(), nil
	}

	// Auto-login (ADR-0011): read the post-commit state and issue a session at the
	// post-bump version, only after the transaction committed — a tx failure above
	// returns the error with no session, leaving the account consistent.
	user, err := ts.Users().Get(ctx, prt.UserID)
	if err != nil {
		return nil, err
	}
	tok, err := s.auth.Issuer().Issue(auth.Principal{
		UserID:            user.ID,
		TenantID:          tenant.ID,
		Role:              sessionRole(user.Role),
		CredentialVersion: user.CredentialVersion,
	})
	if err != nil {
		return nil, err
	}
	return gen.ConfirmPasswordReset200JSONResponse{
		AccessToken: tok,
		TokenType:   gen.Bearer,
		ExpiresIn:   int(s.auth.Issuer().AccessTTL().Seconds()),
	}, nil
}

// invalidResetToken is the single generic 400 for every unusable token — absent,
// expired, or already used — so the response never distinguishes which.
func invalidResetToken() gen.ConfirmPasswordReset400ApplicationProblemPlusJSONResponse {
	return gen.ConfirmPasswordReset400ApplicationProblemPlusJSONResponse{
		BadRequestApplicationProblemPlusJSONResponse: badRequest("this reset link is invalid or has expired"),
	}
}
