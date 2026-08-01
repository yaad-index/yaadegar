package api

import (
	"context"
	"errors"
	"fmt"

	"github.com/yaad-index/yaadegar/internal/api/gen"
	"github.com/yaad-index/yaadegar/internal/storage"
)

// Admin user management (ADR-0009 Cut 1). These endpoints live on the instance
// admin surface (/admin/*), already gated by requireAdmin — which authorizes the
// instance-admin capability (an is_admin owner, ADR-0010), not a separate identity.

// AdminListTenants returns a page of all tenants for the admin browse.
func (s *Server) AdminListTenants(ctx context.Context, req gen.AdminListTenantsRequestObject) (gen.AdminListTenantsResponseObject, error) {
	tenants, total, err := s.store.ListTenants(ctx, pageParams(req.Params.Limit, req.Params.Offset))
	if err != nil {
		return nil, err
	}
	items := make([]gen.AdminTenant, 0, len(tenants))
	for _, t := range tenants {
		items = append(items, gen.AdminTenant{Id: t.ID, Subdomain: t.Subdomain})
	}
	return gen.AdminListTenants200JSONResponse{Items: items, Total: total}, nil
}

// AdminListUsers returns a page of a tenant's users.
func (s *Server) AdminListUsers(ctx context.Context, req gen.AdminListUsersRequestObject) (gen.AdminListUsersResponseObject, error) {
	tenant, err := s.store.TenantByID(ctx, req.TenantId)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return gen.AdminListUsers404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse: notFound("tenant not found"),
			}, nil
		}
		return nil, err
	}
	users, total, err := s.store.ForTenant(tenant).Users().List(ctx, pageParams(req.Params.Limit, req.Params.Offset))
	if err != nil {
		return nil, err
	}
	items := make([]gen.AdminUser, 0, len(users))
	for _, u := range users {
		items = append(items, toAdminUser(u))
	}
	return gen.AdminListUsers200JSONResponse{Items: items, Total: total}, nil
}

// AdminCreateUser provisions an owner or giver by email with no credential; the
// user sets one later via an enabled login method (ADR-0009).
func (s *Server) AdminCreateUser(ctx context.Context, req gen.AdminCreateUserRequestObject) (gen.AdminCreateUserResponseObject, error) {
	if req.Body == nil || req.Body.Email == "" {
		return gen.AdminCreateUser400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: badRequest("email is required"),
		}, nil
	}
	role, ok := toStorageRole(req.Body.Role)
	if !ok {
		return gen.AdminCreateUser400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: badRequest("role must be owner or giver"),
		}, nil
	}
	tenant, err := s.store.TenantByID(ctx, req.TenantId)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return gen.AdminCreateUser404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse: notFound("tenant not found"),
			}, nil
		}
		return nil, err
	}
	email := req.Body.Email
	name := email
	if req.Body.Name != nil && *req.Body.Name != "" {
		name = *req.Body.Name
	}
	user, err := s.store.ForTenant(tenant).Users().Create(ctx, storage.User{
		Name:     name,
		Email:    email,
		Username: &email, // the email doubles as the login handle
		Role:     role,
	})
	if err != nil {
		if errors.Is(err, storage.ErrConflict) {
			return gen.AdminCreateUser409ApplicationProblemPlusJSONResponse{
				ConflictApplicationProblemPlusJSONResponse: conflict("a user with that email already exists in this tenant"),
			}, nil
		}
		return nil, err
	}
	return gen.AdminCreateUser201JSONResponse(toAdminUser(user)), nil
}

// AdminUpdateUser changes a user's role and/or ban flag. Demotion to giver is
// rejected while the account owns lists (owner access flows through list
// ownership), naming the blocking count so the admin can act (ADR-0009 Cut 1).
func (s *Server) AdminUpdateUser(ctx context.Context, req gen.AdminUpdateUserRequestObject) (gen.AdminUpdateUserResponseObject, error) {
	if req.Body == nil || (req.Body.Role == nil && req.Body.Banned == nil) {
		return gen.AdminUpdateUser400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: badRequest("provide role and/or banned"),
		}, nil
	}
	tenant, err := s.store.TenantByID(ctx, req.TenantId)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return gen.AdminUpdateUser404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse: notFound("tenant not found"),
			}, nil
		}
		return nil, err
	}
	ts := s.store.ForTenant(tenant)
	user, err := ts.Users().Get(ctx, req.UserId)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return gen.AdminUpdateUser404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse: notFound("user not found"),
			}, nil
		}
		return nil, err
	}

	if req.Body.Role != nil {
		role, ok := toStorageRole(*req.Body.Role)
		if !ok {
			return gen.AdminUpdateUser400ApplicationProblemPlusJSONResponse{
				BadRequestApplicationProblemPlusJSONResponse: badRequest("role must be owner or giver"),
			}, nil
		}
		// Demotion precondition: an owner being demoted to giver must own no lists,
		// or the demoted account would retain owner access via list_owners.
		if user.Role == storage.RoleOwner && role == storage.RoleGiver {
			_, owned, err := ts.Lists().List(ctx, user.ID, storage.Page{Limit: 1})
			if err != nil {
				return nil, err
			}
			if owned > 0 {
				return gen.AdminUpdateUser409ApplicationProblemPlusJSONResponse(problemDetail(409, fmt.Sprintf(
					"cannot demote to giver: this account still owns %d list(s); reassign or delete them first",
					owned))), nil
			}
		}
		if err := ts.Users().SetRole(ctx, user.ID, role); err != nil {
			return nil, err
		}
		user.Role = role
	}

	if req.Body.Banned != nil {
		if err := ts.Users().SetBanned(ctx, user.ID, *req.Body.Banned); err != nil {
			return nil, err
		}
		user.Banned = *req.Body.Banned
	}

	return gen.AdminUpdateUser200JSONResponse(toAdminUser(user)), nil
}

// toAdminUser maps a stored user to the admin API view (no credential fields).
func toAdminUser(u storage.User) gen.AdminUser {
	au := gen.AdminUser{
		Id:       u.ID,
		TenantId: u.TenantID,
		Email:    u.Email,
		Role:     gen.AdminUserRole(u.Role),
		Banned:   u.Banned,
		IsAdmin:  u.IsAdmin,
	}
	if u.Name != "" {
		au.Name = ptr(u.Name)
	}
	return au
}

// toStorageRole validates and maps the API role enum to the storage role.
func toStorageRole(r gen.AdminUserRole) (storage.UserRole, bool) {
	switch r {
	case gen.Owner:
		return storage.RoleOwner, true
	case gen.Giver:
		return storage.RoleGiver, true
	default:
		return "", false
	}
}
