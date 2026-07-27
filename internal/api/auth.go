package api

import (
	"context"
	"errors"

	"github.com/yaad-index/yaadegar/internal/api/gen"
	"github.com/yaad-index/yaadegar/internal/auth"
	"github.com/yaad-index/yaadegar/internal/storage"
)

// Login exchanges username+password for a session access token (ADR-0005 §3). It
// is unauthenticated and tenant-scoped by Host. A wrong username, wrong password,
// or a user with no password credential all return the same 401 so the response
// never reveals which; when the password method is disabled it returns 404.
func (s *Server) Login(ctx context.Context, req gen.LoginRequestObject) (gen.LoginResponseObject, error) {
	if !s.auth.Enabled(auth.MethodPassword) {
		return gen.Login404ApplicationProblemPlusJSONResponse(
			problemDetail(404, "password login is not enabled on this instance"),
		), nil
	}
	ts, tenant, ok := s.tenantStore(ctx)
	if !ok {
		return nil, errMissingContext
	}
	if req.Body == nil || req.Body.Username == "" || req.Body.Password == "" {
		return gen.Login400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: badRequest("username and password are required"),
		}, nil
	}

	user, err := ts.Users().ByUsername(ctx, req.Body.Username)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return unauthorizedLogin(), nil
		}
		return nil, err
	}
	if user.PasswordHash == "" {
		return unauthorizedLogin(), nil
	}
	okPw, err := auth.VerifyPassword(req.Body.Password, user.PasswordHash)
	if err != nil {
		// A malformed stored hash is a server-side problem, not a client 401.
		return nil, err
	}
	if !okPw {
		return unauthorizedLogin(), nil
	}

	token, err := s.auth.Issuer().Issue(auth.Principal{
		UserID:   user.ID,
		TenantID: tenant.ID,
		Role:     auth.RoleOwner,
	})
	if err != nil {
		return nil, err
	}
	return gen.Login200JSONResponse{
		AccessToken: token,
		TokenType:   gen.Bearer,
		ExpiresIn:   int(s.auth.Issuer().AccessTTL().Seconds()),
	}, nil
}

// unauthorizedLogin is the single 401 used for every credential failure, so the
// response never distinguishes unknown-user from wrong-password.
func unauthorizedLogin() gen.Login401ApplicationProblemPlusJSONResponse {
	return gen.Login401ApplicationProblemPlusJSONResponse{
		UnauthorizedApplicationProblemPlusJSONResponse: unauthorized("invalid username or password"),
	}
}
