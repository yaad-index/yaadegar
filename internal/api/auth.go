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

	// Brute-force guard: fail closed on a rate-limited IP or identity before any
	// credential work. The identity key is tenant-scoped so tenants don't share a
	// bucket for the same username.
	ipKey, idKey := s.loginKeys(ctx, "owner:"+tenant.ID, req.Body.Username)
	if !s.loginAllowed(ipKey, idKey) {
		return gen.Login429ApplicationProblemPlusJSONResponse{
			TooManyRequestsApplicationProblemPlusJSONResponse: tooManyRequests(),
		}, nil
	}

	user, err := ts.Users().ByUsername(ctx, req.Body.Username)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			auth.VerifyDummy(req.Body.Password) // constant-time vs. a real account (#62)
			s.loginFailed(ipKey, idKey)
			return unauthorizedLogin(), nil
		}
		return nil, err
	}
	if user.PasswordHash == "" {
		auth.VerifyDummy(req.Body.Password)
		s.loginFailed(ipKey, idKey)
		return unauthorizedLogin(), nil
	}
	okPw, err := auth.VerifyPassword(req.Body.Password, user.PasswordHash)
	if err != nil {
		// A malformed stored hash is a server-side problem, not a client 401.
		return nil, err
	}
	if !okPw {
		s.loginFailed(ipKey, idKey)
		return unauthorizedLogin(), nil
	}
	// Ban enforced at token issue (ADR-0009): a banned account gets no session. Not
	// counted as a credential failure — the password was correct.
	if user.Banned {
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
	s.loginSucceeded(ipKey, idKey)
	return gen.Login200JSONResponse{
		AccessToken: token,
		TokenType:   gen.Bearer,
		ExpiresIn:   int(s.auth.Issuer().AccessTTL().Seconds()),
	}, nil
}

// GetAuthMethods reports which owner-login affordances the frontend should render
// for the tenant resolved from the Host (ADR-0008 Cut 2). It is unauthenticated.
// Owner login lives only on the main instance domain: on a custom/CNAME domain
// (public-giver-only) both methods are false and login_url carries the tenant's
// canonical subdomain login page so the frontend can redirect an owner there.
func (s *Server) GetAuthMethods(ctx context.Context, _ gen.GetAuthMethodsRequestObject) (gen.GetAuthMethodsResponseObject, error) {
	tenant, ok := tenantFromContext(ctx)
	if !ok {
		return nil, errMissingContext
	}
	custom := s.isCustomDomainHost(routingHostFromContext(ctx))
	loginURL := ""
	if s.baseDomain != "" {
		loginURL = "https://" + tenant.Subdomain + "." + s.baseDomain + "/login"
	}
	return gen.GetAuthMethods200JSONResponse{
		Password: s.auth.Enabled(auth.MethodPassword) && !custom,
		Google:   s.oauth != nil && tenant.OAuthGoogleEnabled && !custom,
		LoginUrl: loginURL,
	}, nil
}

// loginKeys returns the (ip, identity) rate-limit keys for a login attempt. The IP
// key is global across surfaces — one attacker, one bucket; the identity key is
// namespaced by surface/scope so distinct principals never share a bucket.
func (s *Server) loginKeys(ctx context.Context, idScope, username string) (ipKey, idKey string) {
	return "ip:" + clientIPFromContext(ctx), "id:" + idScope + ":" + username
}

// loginAllowed reports whether both the IP and the identity are under their limits.
func (s *Server) loginAllowed(ipKey, idKey string) bool {
	return s.loginLimiter.Allow(ipKey) && s.loginLimiter.Allow(idKey)
}

// loginFailed / loginSucceeded record the attempt outcome against both keys.
func (s *Server) loginFailed(ipKey, idKey string) {
	s.loginLimiter.RecordFailure(ipKey)
	s.loginLimiter.RecordFailure(idKey)
}

func (s *Server) loginSucceeded(ipKey, idKey string) {
	s.loginLimiter.RecordSuccess(ipKey)
	s.loginLimiter.RecordSuccess(idKey)
}

// unauthorizedLogin is the single 401 used for every credential failure, so the
// response never distinguishes unknown-user from wrong-password.
func unauthorizedLogin() gen.Login401ApplicationProblemPlusJSONResponse {
	return gen.Login401ApplicationProblemPlusJSONResponse{
		UnauthorizedApplicationProblemPlusJSONResponse: unauthorized("invalid username or password"),
	}
}
