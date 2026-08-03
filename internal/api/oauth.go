package api

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/yaad-index/yaadegar/internal/auth"
	"github.com/yaad-index/yaadegar/internal/oauthlogin"
	"github.com/yaad-index/yaadegar/internal/storage"
)

// OAuth endpoint paths (ADR-0008 §2–3). start + callback live on the fixed
// redirect host; complete lives on the tenant host. All three sit under
// /api/v1/auth/oauth/, which the owner-auth and tenant-resolution middleware skip
// — each endpoint carries its own tenant (in the signed state, then the ticket),
// never trusting a request-supplied host.
const (
	oauthStartPath    = "/api/v1/auth/oauth/google/start"
	oauthCallbackPath = "/api/v1/auth/oauth/google/callback"
	oauthCompletePath = "/api/v1/auth/oauth/complete"
	// oauthCookiePath scopes the state cookie to the start/callback pair on the
	// fixed host; it is never sent anywhere else.
	oauthCookiePath = "/api/v1/auth/oauth"
)

// Cookie names and lifetimes for the flow.
const (
	oauthStateCookie = "yaadegar_oauth_state"
	// sessionCookie carries the owner session JWT set by /complete on the tenant
	// host (ADR-0008 §3). It is host-scoped (no Domain attribute) so a session never
	// escapes its tenant host.
	sessionCookie  = "yaadegar_session"
	oauthStateTTL  = 10 * time.Minute
	oauthTicketTTL = 1 * time.Minute
)

// oauth_error codes carried to the login page for user-facing callback failures
// (#124). They are stable, non-secret slugs the frontend maps to friendly copy;
// the provider may also supply its own (e.g. access_denied), which passes through.
const (
	oauthErrEmailUnverified = "email_unverified"
	oauthErrNoOwner         = "no_owner"
	oauthErrAlreadyLinked   = "already_linked"
	oauthErrSigninFailed    = "signin_failed"
)

// errNoOwnerForEmail / errOwnerAlreadyLinked are the two link-only rejections
// (ADR-0008 §5). They are distinct so the callback can return a clear, actionable
// status without leaking which owners exist beyond the caller's own verified email.
var (
	errNoOwnerForEmail    = errors.New("oauth: no owner with this verified email in the tenant")
	errOwnerAlreadyLinked = errors.New("oauth: owner already linked to a different provider subject")
)

// oauthConfigured reports whether the instance has a Google client wired up. When
// false every endpoint is a 404 — the method is simply absent (ADR-0008 §7).
func (s *Server) oauthConfigured(w http.ResponseWriter) bool {
	if s.oauth == nil {
		writeProblem(w, http.StatusNotFound, "google login is not enabled on this instance")
		return false
	}
	return true
}

// handleOAuthStart begins the flow (fixed host): it validates the tenant hint,
// gates on the per-tenant toggle, mints state + nonce + PKCE verifier into a
// signed host-scoped cookie, and redirects to the provider.
func (s *Server) handleOAuthStart(w http.ResponseWriter, r *http.Request) {
	if !s.oauthConfigured(w) {
		return
	}
	q := r.URL.Query()
	tenantHost := hostname(q.Get("tenant"))
	if tenantHost == "" {
		writeProblem(w, http.StatusBadRequest, "the tenant parameter is required")
		return
	}
	tenant, err := s.tenantForHost(r.Context(), tenantHost)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeProblem(w, http.StatusNotFound, "no tenant is configured for this host")
			return
		}
		s.logger.Error("oauth start: tenant resolution failed", "err", err, "host", tenantHost)
		writeProblem(w, http.StatusInternalServerError, "internal error")
		return
	}
	// Owner login is main-domain-only: a custom/CNAME domain is public-giver-only,
	// so it never starts an owner login (ADR-0008 Cut 2). Defense in depth behind
	// the frontend's methods-gated affordance.
	if s.isCustomDomainHost(tenantHost) {
		writeProblem(w, http.StatusNotFound, "owner login is not available on this domain")
		return
	}
	if !tenant.OAuthGoogleEnabled {
		writeProblem(w, http.StatusNotFound, "google login is not enabled for this tenant")
		return
	}

	state := oauthlogin.RandomToken(24)
	nonce := oauthlogin.RandomToken(24)
	verifier := oauthlogin.NewPKCEVerifier()
	payload := oauthlogin.StatePayload{
		State:        state,
		Nonce:        nonce,
		PKCEVerifier: verifier,
		TenantID:     tenant.ID,
		TenantHost:   tenantHost,
		ReturnTo:     sanitizeReturnTo(q.Get("return_to")),
		Exp:          s.clock.Now().Add(oauthStateTTL).Unix(),
	}
	http.SetCookie(w, &http.Cookie{
		Name:     oauthStateCookie,
		Value:    s.oauth.Signer().SignState(payload),
		Path:     oauthCookiePath,
		MaxAge:   int(oauthStateTTL.Seconds()),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, s.oauth.AuthCodeURL(state, nonce, verifier), http.StatusFound)
}

// handleOAuthCallback lands on the fixed host: it validates the signed state
// (CSRF), exchanges the code with the PKCE verifier, verifies the ID token, runs
// the link-only account model, and bounces a one-time ticket to the tenant host.
func (s *Server) handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	if !s.oauthConfigured(w) {
		return
	}
	ck, err := r.Cookie(oauthStateCookie)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "missing or expired login state")
		return
	}
	st, err := s.oauth.Signer().VerifyState(ck.Value)
	// The state cookie is single-purpose; clear it regardless of the outcome.
	s.clearCookie(w, oauthStateCookie, oauthCookiePath)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid login state")
		return
	}
	if s.clock.Now().Unix() > st.Exp {
		writeProblem(w, http.StatusBadRequest, "login state expired; start again")
		return
	}

	q := r.URL.Query()
	if provErr := q.Get("error"); provErr != "" {
		// A normal outcome (e.g. the user cancelled at Google) — return them to the
		// login page with a readable message rather than an API error.
		s.logger.Info("oauth callback: provider returned error", "error", provErr)
		s.redirectLoginError(w, r, st.TenantHost, provErr)
		return
	}
	if subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(st.State)) != 1 {
		writeProblem(w, http.StatusBadRequest, "state mismatch")
		return
	}
	code := q.Get("code")
	if code == "" {
		writeProblem(w, http.StatusBadRequest, "missing authorization code")
		return
	}

	rawID, err := s.oauth.Exchange(r.Context(), code, st.PKCEVerifier)
	if err != nil {
		// Transport/provider failure — logged server-side; the giver just sees a
		// "try again" on the login page, not a raw 502.
		s.logger.Error("oauth callback: code exchange failed", "err", err)
		s.redirectLoginError(w, r, st.TenantHost, oauthErrSigninFailed)
		return
	}
	id, err := s.oauth.VerifyIDToken(r.Context(), rawID, st.Nonce)
	if err != nil {
		s.logger.Warn("oauth callback: id_token verification failed", "err", err)
		s.redirectLoginError(w, r, st.TenantHost, oauthErrSigninFailed)
		return
	}
	// Guard 1: the provider must assert the email is verified (ADR-0008 §5). This
	// is the check that closes the email-collision path.
	if !id.EmailVerified || id.Email == "" {
		s.redirectLoginError(w, r, st.TenantHost, oauthErrEmailUnverified)
		return
	}

	tenant, err := s.store.TenantByID(r.Context(), st.TenantID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeProblem(w, http.StatusNotFound, "no tenant is configured for this login")
			return
		}
		s.logger.Error("oauth callback: tenant load failed", "err", err)
		writeProblem(w, http.StatusInternalServerError, "internal error")
		return
	}
	userID, err := s.resolveOAuthOwner(r.Context(), s.store.ForTenant(tenant), id)
	switch {
	case err == nil:
	case errors.Is(err, errNoOwnerForEmail):
		s.redirectLoginError(w, r, st.TenantHost, oauthErrNoOwner)
		return
	case errors.Is(err, errOwnerAlreadyLinked):
		s.redirectLoginError(w, r, st.TenantHost, oauthErrAlreadyLinked)
		return
	default:
		// Genuine server fault — no meaningful user-facing page, so problem+json.
		s.logger.Error("oauth callback: owner resolution failed", "err", err)
		writeProblem(w, http.StatusInternalServerError, "internal error")
		return
	}

	ticket := oauthlogin.Ticket{
		TenantID: tenant.ID,
		UserID:   userID,
		ReturnTo: st.ReturnTo,
		Jti:      oauthlogin.RandomToken(16),
		Exp:      s.clock.Now().Add(oauthTicketTTL).Unix(),
	}
	s.redirectTenant(w, r, st.TenantHost,
		oauthCompletePath+"?ticket="+url.QueryEscape(s.oauth.Signer().SignTicket(ticket)))
}

// handleOAuthComplete lands on the tenant host: it validates and single-use-
// consumes the ticket, mints the normal owner session (ADR-0005 issuer), sets the
// host-scoped session cookie, and returns the browser to the app.
func (s *Server) handleOAuthComplete(w http.ResponseWriter, r *http.Request) {
	if !s.oauthConfigured(w) {
		return
	}
	raw := r.URL.Query().Get("ticket")
	if raw == "" {
		writeProblem(w, http.StatusBadRequest, "missing ticket")
		return
	}
	t, err := s.oauth.Signer().VerifyTicket(raw)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid ticket")
		return
	}
	exp := time.Unix(t.Exp, 0)
	if !s.clock.Now().Before(exp) {
		writeProblem(w, http.StatusBadRequest, "ticket expired; sign in again")
		return
	}
	// One-time use: the first presentation wins, every later one is rejected.
	if !s.ticketGuard.Consume(t.Jti, exp) {
		writeProblem(w, http.StatusBadRequest, "ticket already used")
		return
	}

	// Load the user's current credential_version so the session pins it (ADR-0011).
	// The ticket only carries identity; a fresh read here keeps the OAuth session on
	// the same invalidation footing as a password login.
	tenant, err := s.store.TenantByID(r.Context(), t.TenantID)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid ticket")
		return
	}
	user, err := s.store.ForTenant(tenant).Users().Get(r.Context(), t.UserID)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid ticket")
		return
	}

	token, err := s.auth.Issuer().Issue(auth.Principal{
		UserID:            t.UserID,
		TenantID:          t.TenantID,
		Role:              auth.RoleOwner,
		CredentialVersion: user.CredentialVersion,
	})
	if err != nil {
		s.logger.Error("oauth complete: issue session failed", "err", err)
		writeProblem(w, http.StatusInternalServerError, "internal error")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		MaxAge:   int(s.auth.Issuer().AccessTTL().Seconds()),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, sanitizeReturnTo(t.ReturnTo), http.StatusFound)
}

// resolveOAuthOwner resolves a Google-verified identity to an account (ADR-0008 §5,
// extended by ADR-0012 cut 2):
//   - an already-linked subject resolves straight to its account (returning login);
//   - a verified email matching an existing account links that account, unless the
//     account is already linked to a different subject;
//   - a verified email with no account self-registers a new one when the instance
//     registration policy allows it, and otherwise stays a link-only rejection.
func (s *Server) resolveOAuthOwner(ctx context.Context, ts storage.TenantStore, id oauthlogin.Identity) (string, error) {
	existing, err := ts.OAuthIdentities().ByProviderSubject(ctx, storage.OAuthProviderGoogle, id.Subject)
	if err == nil {
		return existing.UserID, nil // returning login: subject already linked
	}
	if !errors.Is(err, storage.ErrNotFound) {
		return "", err
	}

	owner, err := ts.Users().ByEmail(ctx, id.Email)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return s.selfRegisterOAuthUser(ctx, ts, id)
		}
		return "", err
	}
	// Guard: reject a second Google account attaching to an owner already linked to
	// a different subject (fold-in — the branch the ADR left implicit).
	link, err := ts.OAuthIdentities().ByUserProvider(ctx, owner.ID, storage.OAuthProviderGoogle)
	switch {
	case err == nil && link.Subject != id.Subject:
		return "", errOwnerAlreadyLinked
	case err != nil && !errors.Is(err, storage.ErrNotFound):
		return "", err
	}

	if _, err := ts.OAuthIdentities().Create(ctx, storage.OAuthIdentity{
		UserID:   owner.ID,
		Provider: storage.OAuthProviderGoogle,
		Subject:  id.Subject,
		Email:    id.Email,
	}); err != nil {
		// A concurrent login may have linked the same subject first; re-read and use it.
		if errors.Is(err, storage.ErrConflict) {
			if e2, e := ts.OAuthIdentities().ByProviderSubject(ctx, storage.OAuthProviderGoogle, id.Subject); e == nil {
				return e2.UserID, nil
			}
		}
		return "", err
	}
	return owner.ID, nil
}

// selfRegisterOAuthUser provisions a new account from a Google-verified identity when
// the instance registration policy allows it (ADR-0012 cut 2). The account is created
// with no password (empty hash) and active immediately — the provider already verified
// the email, so this path mints no verification token and runs no captcha — at the role
// the policy dictates (giver or owner), and the provider subject is linked so the next
// login resolves straight through. When self-registration is disabled this returns the
// same link-only rejection as before cut 2 (errNoOwnerForEmail). A later password is
// self-servable through the forgot-password flow, whose resettable() guard covers a
// no-password account.
func (s *Server) selfRegisterOAuthUser(ctx context.Context, ts storage.TenantStore, id oauthlogin.Identity) (string, error) {
	role, enabled := registrationRole(s.registrationPolicy)
	if !enabled {
		return "", errNoOwnerForEmail
	}

	username := id.Email // the email doubles as the login handle, matching email register
	user, err := ts.Users().Create(ctx, storage.User{
		Name:     id.Email,
		Email:    id.Email,
		Username: &username,
		Role:     role,
		Status:   storage.UserStatusActive,
	})
	if err != nil {
		// A concurrent OAuth (or email) register for the same email may have created it
		// first; fall back to that account and link this subject to it.
		if errors.Is(err, storage.ErrConflict) {
			if existing, e := ts.Users().ByEmail(ctx, id.Email); e == nil {
				user = existing
			} else {
				return "", err
			}
		} else {
			return "", err
		}
	}

	if _, err := ts.OAuthIdentities().Create(ctx, storage.OAuthIdentity{
		UserID:   user.ID,
		Provider: storage.OAuthProviderGoogle,
		Subject:  id.Subject,
		Email:    id.Email,
	}); err != nil {
		// A concurrent login may have linked the same subject first; re-read and use it.
		if errors.Is(err, storage.ErrConflict) {
			if e2, e := ts.OAuthIdentities().ByProviderSubject(ctx, storage.OAuthProviderGoogle, id.Subject); e == nil {
				return e2.UserID, nil
			}
		}
		return "", err
	}
	return user.ID, nil
}

// redirectTenant sends the browser to a local path on the tenant host over https
// (ADR-0008: the tenant host was validated at /start and carried in the signed
// state, so it is not attacker-controlled). path must already be a local absolute
// path, optionally with a query.
func (s *Server) redirectTenant(w http.ResponseWriter, r *http.Request, tenantHost, path string) {
	http.Redirect(w, r, "https://"+tenantHost+path, http.StatusFound)
}

// redirectLoginError bounces a user-facing callback failure back to the tenant
// login page carrying an oauth_error code (#124), instead of a raw problem+json
// body. It targets /login directly — not "/", which the app layout would redirect
// to /login on its own and drop the query — so the message survives to the page,
// where it is rendered as escaped text.
func (s *Server) redirectLoginError(w http.ResponseWriter, r *http.Request, tenantHost, code string) {
	s.redirectTenant(w, r, tenantHost, "/login?oauth_error="+url.QueryEscape(code))
}

// clearCookie expires a cookie by name/path.
func (s *Server) clearCookie(w http.ResponseWriter, name, path string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     path,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

// sanitizeReturnTo bounds a post-login redirect to a local absolute path, so a
// crafted return_to can never bounce the browser to an external origin. Anything
// that is not a single-slash-rooted path collapses to "/".
func sanitizeReturnTo(p string) string {
	if p == "" || !strings.HasPrefix(p, "/") || strings.HasPrefix(p, "//") {
		return "/"
	}
	return p
}
