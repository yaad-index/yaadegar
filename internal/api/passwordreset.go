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
// enumeration-safe: the response is ALWAYS a 202 with no body, whether or not the
// identifier resolves to an account. Two things keep existence from leaking:
//   - the token mint (crypto/rand + hash) runs on every request, found or not, so
//     that constant work is not a tell;
//   - the email is sent off the response path (a goroutine), so response latency
//     never depends on whether — or how slowly — an email went out.
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

	// Mint the token up front so the same crypto work is spent on every request.
	raw, hash, err := token.New()
	if err != nil {
		return nil, err
	}

	if user, found := s.lookupResettable(ctx, ts, req.Body.Identifier); found {
		expires := s.clock.Now().Add(passwordResetTTL)
		if _, err := ts.PasswordResetTokens().Create(ctx, storage.PasswordResetToken{
			UserID: user.ID, TokenHash: hash, ExpiresAt: expires,
		}); err != nil {
			// Log, but still return the identical 202 — a server-side hiccup must not be
			// an observable difference either.
			s.logger.ErrorContext(ctx, "password reset: persist token failed", "error", err)
		} else {
			s.sendResetEmailAsync(tenant, user.Email, raw)
		}
	}

	// Always the same: 202, no body, no hint about what happened.
	return gen.RequestPasswordReset202Response{}, nil
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

// sendResetEmailAsync emails the reset link off the request path so response timing
// never depends on delivery. Failures are logged, never surfaced (that would leak
// existence).
func (s *Server) sendResetEmailAsync(tenant storage.Tenant, to, rawToken string) {
	link := s.resetLink(tenant, rawToken)
	go func() {
		ctx := context.Background()
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

	// Claim the token atomically: only the winner of a concurrent confirm proceeds.
	claimed, err := ts.PasswordResetTokens().MarkUsed(ctx, prt.ID, now)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return invalidResetToken(), nil
		}
		return nil, err
	}
	if !claimed {
		return invalidResetToken(), nil
	}

	// Set the new password — bumps credential_version, invalidating every session.
	if err := ts.Users().SetPasswordHash(ctx, prt.UserID, hash); err != nil {
		return nil, err
	}

	// Auto-login (ADR-0011): issue a session at the post-bump version so the caller
	// lands logged in on the device that completed the reset.
	user, err := ts.Users().Get(ctx, prt.UserID)
	if err != nil {
		return nil, err
	}
	// Establishing a password through an emailed link proves email ownership, so it
	// also completes activation for a still-pending account (ADR-0012 cut 1b) — the
	// same proof the verification link would give. Without this, a pending account that
	// set its password here would auto-login once but then be unable to log in again
	// (the login gate rejects pending). A no-op for an already-active account.
	if user.Status == storage.UserStatusPending {
		if err := ts.Users().SetStatus(ctx, user.ID, storage.UserStatusActive); err != nil {
			return nil, err
		}
		user.Status = storage.UserStatusActive
	}
	tok, err := s.auth.Issuer().Issue(auth.Principal{
		UserID:            user.ID,
		TenantID:          tenant.ID,
		Role:              auth.RoleOwner,
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
