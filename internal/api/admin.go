package api

import (
	"context"
	"errors"

	"github.com/yaad-index/yaadegar/internal/api/gen"
	"github.com/yaad-index/yaadegar/internal/auth"
	"github.com/yaad-index/yaadegar/internal/storage"
)

// AdminLogin authenticates the instance superadmin and issues a session token
// carrying role=superadmin and the sentinel tenant id (ADR-0005 §6). It is
// unauthenticated and not tenant-scoped; when no superadmin is configured the
// admin surface is disabled and this reports 404.
func (s *Server) AdminLogin(ctx context.Context, req gen.AdminLoginRequestObject) (gen.AdminLoginResponseObject, error) {
	if !s.adminEnabled {
		return gen.AdminLogin404ApplicationProblemPlusJSONResponse(
			problemDetail(404, "the admin surface is not enabled on this instance"),
		), nil
	}
	if req.Body == nil || req.Body.Username == "" || req.Body.Password == "" {
		return gen.AdminLogin400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: badRequest("username and password are required"),
		}, nil
	}

	admin, err := s.store.AdminByUsername(ctx, req.Body.Username)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return adminUnauthorized(), nil
		}
		return nil, err
	}
	okPw, err := auth.VerifyPassword(req.Body.Password, admin.PasswordHash)
	if err != nil {
		return nil, err
	}
	if !okPw {
		return adminUnauthorized(), nil
	}

	token, err := s.auth.Issuer().Issue(auth.Principal{
		UserID:   admin.ID,
		TenantID: auth.SuperadminTenant,
		Role:     auth.RoleSuperadmin,
	})
	if err != nil {
		return nil, err
	}
	return gen.AdminLogin200JSONResponse{
		AccessToken: token,
		TokenType:   gen.Bearer,
		ExpiresIn:   int(s.auth.Issuer().AccessTTL().Seconds()),
	}, nil
}

// AdminGetMe returns the authenticated superadmin. requireAdmin has already
// enforced the surface being enabled and the superadmin role.
func (s *Server) AdminGetMe(ctx context.Context, _ gen.AdminGetMeRequestObject) (gen.AdminGetMeResponseObject, error) {
	admin, ok := adminFromContext(ctx)
	if !ok {
		return nil, errMissingContext
	}
	return gen.AdminGetMe200JSONResponse{Id: ptr(admin.ID), Username: ptr(admin.Username)}, nil
}

// AdminCreateTenant provisions a tenant over HTTP (the superadmin equivalent of
// the create-tenant CLI). requireAdmin has already enforced the surface and role.
func (s *Server) AdminCreateTenant(ctx context.Context, req gen.AdminCreateTenantRequestObject) (gen.AdminCreateTenantResponseObject, error) {
	if req.Body == nil || req.Body.Subdomain == "" {
		return gen.AdminCreateTenant400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: badRequest("subdomain is required"),
		}, nil
	}
	tenant, err := s.store.CreateTenant(ctx, storage.Tenant{Subdomain: req.Body.Subdomain})
	if err != nil {
		switch {
		case errors.Is(err, storage.ErrConflict):
			return gen.AdminCreateTenant409ApplicationProblemPlusJSONResponse{
				ConflictApplicationProblemPlusJSONResponse: conflict("a tenant with that subdomain already exists"),
			}, nil
		case errors.Is(err, storage.ErrInvalidSubdomain):
			return gen.AdminCreateTenant400ApplicationProblemPlusJSONResponse{
				BadRequestApplicationProblemPlusJSONResponse: badRequest("invalid subdomain"),
			}, nil
		}
		return nil, err
	}
	return gen.AdminCreateTenant201JSONResponse{Id: ptr(tenant.ID), Subdomain: ptr(tenant.Subdomain)}, nil
}

// AdminCreateOwner provisions a password-credentialed owner in a tenant (the
// superadmin equivalent of create-owner). The email doubles as the login username;
// the password is hashed server-side and never echoed back.
func (s *Server) AdminCreateOwner(ctx context.Context, req gen.AdminCreateOwnerRequestObject) (gen.AdminCreateOwnerResponseObject, error) {
	if req.Body == nil || req.Body.TenantId == "" || req.Body.Email == "" || req.Body.Password == "" {
		return gen.AdminCreateOwner400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: badRequest("tenant_id, email, and password are required"),
		}, nil
	}
	tenant, err := s.store.TenantByID(ctx, req.Body.TenantId)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return gen.AdminCreateOwner404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse: notFound("tenant not found"),
			}, nil
		}
		return nil, err
	}
	hash, err := auth.HashPassword(req.Body.Password)
	if err != nil {
		return nil, err
	}
	email := req.Body.Email
	owner, err := s.store.ForTenant(tenant).Users().Create(ctx, storage.User{
		Name:         email,
		Email:        email,
		Username:     &email, // the email doubles as the login username
		PasswordHash: hash,
	})
	if err != nil {
		if errors.Is(err, storage.ErrConflict) {
			return gen.AdminCreateOwner409ApplicationProblemPlusJSONResponse{
				ConflictApplicationProblemPlusJSONResponse: conflict("an owner with that email already exists in this tenant"),
			}, nil
		}
		return nil, err
	}
	return gen.AdminCreateOwner201JSONResponse{
		Id: ptr(owner.ID), TenantId: ptr(tenant.ID), Email: ptr(owner.Email),
	}, nil
}

// adminUnauthorized is the single 401 for a failed admin login, so it never
// distinguishes unknown-admin from wrong-password.
func adminUnauthorized() gen.AdminLogin401ApplicationProblemPlusJSONResponse {
	return gen.AdminLogin401ApplicationProblemPlusJSONResponse{
		UnauthorizedApplicationProblemPlusJSONResponse: unauthorized("invalid username or password"),
	}
}
