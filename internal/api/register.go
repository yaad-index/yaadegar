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

// emailVerificationTTL is how long an emailed verification link stays valid
// (ADR-0012 cut 1a): short, single-use, so a leaked-then-expired link is inert.
const emailVerificationTTL = time.Hour

// Register starts email self-registration (ADR-0012 cut 1a). It is gated by the
// instance registration policy (disabled → 403) and, when enabled, is
// enumeration-safe: the response is ALWAYS a 202 with no body, whether or not the
// email already resolves to an account. The same guards the password-reset flow
// uses keep existence from leaking — a token is minted on every accepted request,
// and the email is sent off the response path (a goroutine), so response timing
// never depends on whether an email went out.
func (s *Server) Register(ctx context.Context, req gen.RegisterRequestObject) (gen.RegisterResponseObject, error) {
	ts, tenant, ok := s.tenantStore(ctx)
	if !ok {
		return nil, errMissingContext
	}

	// Fail-closed: self-registration is off unless the operator opted in (empty
	// policy behaves as disabled). Enforced server-side regardless of the frontend.
	role, enabled := registrationRole(s.registrationPolicy)
	if !enabled {
		return gen.Register403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden("self-registration is not enabled on this instance"),
		}, nil
	}

	if req.Body == nil || req.Body.Email == "" || req.Body.Password == "" {
		return gen.Register400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: badRequest("email and password are required"),
		}, nil
	}

	// Human-challenge gate on the register path only. The default verifier is a no-op
	// (accepts any value); a real provider slots in behind the same interface.
	captchaToken := ""
	if req.Body.CaptchaToken != nil {
		captchaToken = *req.Body.CaptchaToken
	}
	human, err := s.captcha.Verify(ctx, captchaToken, clientIPFromContext(ctx))
	if err != nil || !human {
		return gen.Register400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: badRequest("captcha verification failed"),
		}, nil
	}

	// Validate the password BEFORE any account/token work so a policy failure is a
	// clear 400 and not an observable side effect. This runs whether or not the email
	// exists, so the constant work is not a tell.
	hash, err := auth.HashNewPassword(req.Body.Password)
	if err != nil {
		if errors.Is(err, auth.ErrPasswordTooShort) {
			return gen.Register400ApplicationProblemPlusJSONResponse{
				BadRequestApplicationProblemPlusJSONResponse: badRequest(
					fmt.Sprintf("the password must be at least %d characters", auth.MinPasswordLen)),
			}, nil
		}
		return nil, err
	}

	// Mint the token up front so the same crypto work is spent on every request.
	raw, tokenHash, err := token.New()
	if err != nil {
		return nil, err
	}

	// Enumeration-safe: only create a pending account + send the email when the email
	// is genuinely new. An already-registered email produces the identical 202 with no
	// second account and no email.
	if _, err := ts.Users().ByEmail(ctx, req.Body.Email); errors.Is(err, storage.ErrNotFound) {
		// The email doubles as the login handle so the account can log in by email
		// once verified (Username backs ByUsername; there is no separate handle here).
		username := req.Body.Email
		user, cerr := ts.Users().Create(ctx, storage.User{
			Name:         req.Body.Email,
			Email:        req.Body.Email,
			Username:     &username,
			PasswordHash: hash,
			Role:         role,
			Status:       storage.UserStatusPending,
		})
		if cerr != nil {
			// A racing concurrent register for the same email can lose the unique-username
			// insert; treat every persist hiccup the same — log and still return 202.
			s.logger.ErrorContext(ctx, "register: create pending account failed", "error", cerr)
		} else if _, terr := ts.EmailVerificationTokens().Create(ctx, storage.EmailVerificationToken{
			UserID: user.ID, TokenHash: tokenHash, ExpiresAt: s.clock.Now().Add(emailVerificationTTL),
		}); terr != nil {
			s.logger.ErrorContext(ctx, "register: persist verification token failed", "error", terr)
		} else {
			s.sendVerificationEmailAsync(tenant, req.Body.Email, raw)
		}
	} else if err != nil {
		// A real lookup error (not ErrNotFound) is a server-side problem; still keep the
		// response uniform by logging rather than surfacing it.
		s.logger.ErrorContext(ctx, "register: lookup existing account failed", "error", err)
	}

	// Always the same: 202, no body, no hint about what happened.
	return gen.Register202Response{}, nil
}

// registrationRole maps the instance policy to the role a self-registered account
// takes, and whether self-registration is enabled at all. An unknown/empty policy is
// treated as disabled (fail-closed).
func registrationRole(p storage.RegistrationPolicy) (storage.UserRole, bool) {
	switch p {
	case storage.RegistrationGiversOnly:
		return storage.RoleGiver, true
	case storage.RegistrationOwnersAllowed:
		return storage.RoleOwner, true
	default:
		return "", false
	}
}

// sendVerificationEmailAsync emails the verification link off the request path so
// response timing never depends on delivery. Failures are logged, never surfaced
// (that would leak existence).
func (s *Server) sendVerificationEmailAsync(tenant storage.Tenant, to, rawToken string) {
	link := s.verifyLink(tenant, rawToken)
	go func() {
		ctx := context.Background()
		if err := s.email.Send(ctx, email.Message{
			To:      to,
			Subject: "Verify your email",
			Body: "Verify your email to finish creating your account: " + link +
				"\n\nThis link can be used once and expires soon. If you didn't request it, you can ignore this email.",
		}); err != nil {
			s.logger.ErrorContext(ctx, "register: send verification email failed", "error", err)
		}
	}()
}

// verifyLink builds the tenant-correct verification URL. It mirrors resetLink: when a
// base domain is configured the link targets the owner's own subdomain host (so the
// verify POST resolves the right tenant by Host); otherwise it falls back to the
// configured public link base. The raw token is base64url — already URL-safe.
func (s *Server) verifyLink(tenant storage.Tenant, rawToken string) string {
	base := s.publicLinkBase
	if s.baseDomain != "" {
		base = "https://" + tenant.Subdomain + "." + s.baseDomain
	}
	return base + "/verify?token=" + rawToken
}

// RegisterVerify completes email self-registration (ADR-0012 cut 1a): it validates
// the single-use verification token, activates the pending account, and auto-issues a
// session so the caller lands logged in. An absent, expired, or already-used token is
// one generic 400.
func (s *Server) RegisterVerify(ctx context.Context, req gen.RegisterVerifyRequestObject) (gen.RegisterVerifyResponseObject, error) {
	ts, tenant, ok := s.tenantStore(ctx)
	if !ok {
		return nil, errMissingContext
	}
	if req.Body == nil || req.Body.Token == "" {
		return gen.RegisterVerify400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: badRequest("token is required"),
		}, nil
	}

	evt, err := ts.EmailVerificationTokens().ByHash(ctx, token.Hash(req.Body.Token))
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return invalidVerificationToken(), nil
		}
		return nil, err
	}
	now := s.clock.Now()
	if evt.UsedAt != nil || !now.Before(evt.ExpiresAt) {
		return invalidVerificationToken(), nil
	}

	// Claim the token atomically: only the winner of a concurrent verify proceeds.
	claimed, err := ts.EmailVerificationTokens().MarkUsed(ctx, evt.ID, now)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return invalidVerificationToken(), nil
		}
		return nil, err
	}
	if !claimed {
		return invalidVerificationToken(), nil
	}

	// Activate the account so it can hold a session (the login gate rejects pending).
	if err := ts.Users().SetStatus(ctx, evt.UserID, storage.UserStatusActive); err != nil {
		return nil, err
	}

	// Auto-login: issue a session so the caller lands logged in on the device that
	// completed verification.
	user, err := ts.Users().Get(ctx, evt.UserID)
	if err != nil {
		return nil, err
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
	return gen.RegisterVerify200JSONResponse{
		AccessToken: tok,
		TokenType:   gen.Bearer,
		ExpiresIn:   int(s.auth.Issuer().AccessTTL().Seconds()),
	}, nil
}

// invalidVerificationToken is the single generic 400 for every unusable token —
// absent, expired, or already used — so the response never distinguishes which.
func invalidVerificationToken() gen.RegisterVerify400ApplicationProblemPlusJSONResponse {
	return gen.RegisterVerify400ApplicationProblemPlusJSONResponse{
		BadRequestApplicationProblemPlusJSONResponse: badRequest("this verification link is invalid or has expired"),
	}
}
