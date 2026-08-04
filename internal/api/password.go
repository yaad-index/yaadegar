package api

import (
	"context"
	"errors"
	"fmt"

	"github.com/yaad-index/yaadegar/internal/api/gen"
	"github.com/yaad-index/yaadegar/internal/auth"
)

// ChangePassword lets an authenticated owner change their own password (ADR-0011
// Decision 2). It verifies the current password, routes the new one through the
// shared HashNewPassword funnel (policy + hash), and stores it — which bumps the
// user's credential_version and so invalidates every OTHER session. Because that
// same bump would invalidate the caller's own token, the response re-issues THIS
// session at the new version, so the owner stays logged in on the device that made
// the change while all their other sessions drop.
func (s *Server) ChangePassword(ctx context.Context, req gen.ChangePasswordRequestObject) (gen.ChangePasswordResponseObject, error) {
	owner, ok := ownerFromContext(ctx)
	ts, tenant, ok2 := s.tenantStore(ctx)
	if !ok || !ok2 {
		return nil, errMissingContext
	}
	if req.Body == nil || req.Body.CurrentPassword == "" || req.Body.NewPassword == "" {
		return gen.ChangePassword400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: badRequest("current_password and new_password are required"),
		}, nil
	}

	// An account with no password credential (e.g. OAuth-only) has nothing to change
	// here; say so plainly rather than compare against an empty hash.
	if owner.PasswordHash == "" {
		return gen.ChangePassword403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden("this account has no password set"),
		}, nil
	}

	okPw, err := auth.VerifyPassword(req.Body.CurrentPassword, owner.PasswordHash)
	if err != nil {
		// A malformed stored hash is a server-side problem, not a client error.
		return nil, err
	}
	if !okPw {
		// Surface the real reason (ADR-0011 / #144's lesson), not a generic error.
		return gen.ChangePassword403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden("the current password is incorrect"),
		}, nil
	}

	// The one funnel: shared policy then hash. A policy failure surfaces its own
	// reason (e.g. too short) rather than a generic 400.
	hash, err := auth.HashNewPassword(req.Body.NewPassword)
	if err != nil {
		if errors.Is(err, auth.ErrPasswordTooShort) {
			return gen.ChangePassword400ApplicationProblemPlusJSONResponse{
				BadRequestApplicationProblemPlusJSONResponse: badRequest(
					fmt.Sprintf("the new password must be at least %d characters", auth.MinPasswordLen)),
			}, nil
		}
		return nil, err
	}

	// Persist — this bumps credential_version in the same write, invalidating every
	// existing session for the user (the caller's current token included).
	if err := ts.Users().SetPasswordHash(ctx, owner.ID, hash); err != nil {
		return nil, err
	}

	// Re-issue THIS session at the authoritative post-bump version so the caller
	// stays logged in on the device that made the change.
	updated, err := ts.Users().Get(ctx, owner.ID)
	if err != nil {
		return nil, err
	}
	token, err := s.auth.Issuer().Issue(auth.Principal{
		UserID:            updated.ID,
		TenantID:          tenant.ID,
		Role:              sessionRole(updated.Role),
		CredentialVersion: updated.CredentialVersion,
	})
	if err != nil {
		return nil, err
	}
	return gen.ChangePassword200JSONResponse{
		AccessToken: token,
		TokenType:   gen.Bearer,
		ExpiresIn:   int(s.auth.Issuer().AccessTTL().Seconds()),
	}, nil
}
