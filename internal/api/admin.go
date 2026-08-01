package api

import (
	"context"
	"errors"

	"github.com/yaad-index/yaadegar/internal/api/gen"
	"github.com/yaad-index/yaadegar/internal/auth"
	"github.com/yaad-index/yaadegar/internal/storage"
)

// AdminCreateTenant provisions a tenant over HTTP (the admin equivalent of the
// create-tenant CLI). requireAdmin has already enforced the instance-admin capability.
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
// admin equivalent of create-owner). The email doubles as the login username;
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
