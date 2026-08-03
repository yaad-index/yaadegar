package api

import (
	"context"

	"github.com/yaad-index/yaadegar/internal/api/gen"
)

// GetTenantSettings returns the tenant-level settings the owner controls
// (ADR-0008 Cut 2): the Google-login toggle plus, read-only, whether the instance
// has a Google client configured (the toggle is inert without one).
func (s *Server) GetTenantSettings(ctx context.Context, _ gen.GetTenantSettingsRequestObject) (gen.GetTenantSettingsResponseObject, error) {
	tenant, ok := tenantFromContext(ctx)
	if !ok {
		return nil, errMissingContext
	}
	return gen.GetTenantSettings200JSONResponse{
		OauthGoogleEnabled:     tenant.OAuthGoogleEnabled,
		GoogleClientConfigured: s.oauth != nil,
	}, nil
}

// UpdateTenantSettings flips the tenant's Google-login toggle. The write targets
// the authenticated owner's own tenant, resolved from the request context (the
// Host-resolved tenant, which requireOwner has already matched to the session's
// tenant claim) — never a tenant named in the request. So an owner can only change
// their own tenant.
func (s *Server) UpdateTenantSettings(ctx context.Context, req gen.UpdateTenantSettingsRequestObject) (gen.UpdateTenantSettingsResponseObject, error) {
	tenant, ok := tenantFromContext(ctx)
	if !ok {
		return nil, errMissingContext
	}
	// Owner-role gate (ADR-0009): tenant settings are an owner-only surface; a giver
	// self-registered account is refused before any body/store work.
	if !hasOwnerRole(ctx) {
		return gen.UpdateTenantSettings403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(ownerRoleRequiredDetail),
		}, nil
	}
	if req.Body == nil {
		return gen.UpdateTenantSettings400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: badRequest("a request body is required"),
		}, nil
	}
	if err := s.store.SetTenantOAuthGoogle(ctx, tenant.ID, req.Body.OauthGoogleEnabled); err != nil {
		return nil, err
	}
	return gen.UpdateTenantSettings200JSONResponse{
		OauthGoogleEnabled:     req.Body.OauthGoogleEnabled,
		GoogleClientConfigured: s.oauth != nil,
	}, nil
}
