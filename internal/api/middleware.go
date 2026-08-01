package api

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/yaad-index/yaadegar/internal/auth"
	"github.com/yaad-index/yaadegar/internal/storage"
)

// resolveTenant resolves the tenant from the request Host and puts it in the
// context for every request except /healthz (which is host-agnostic ops). An
// unknown host is a 404 — the request names no tenant we serve.
func (s *Server) resolveTenant(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// /healthz is host-agnostic ops; /admin is the instance-level admin surface
		// and is deliberately NOT tenant-scoped (ADR-0010). The OAuth
		// endpoints (ADR-0008) are host-agnostic too: start/callback run on the fixed
		// redirect host (no tenant) and carry their tenant in the signed state, then
		// the ticket — so they resolve the tenant themselves, not from the Host.
		if r.URL.Path == "/healthz" ||
			strings.HasPrefix(r.URL.Path, "/admin/") ||
			strings.HasPrefix(r.URL.Path, "/api/v1/auth/oauth/") {
			next.ServeHTTP(w, r)
			return
		}
		routingHost := s.hostForRouting(r)
		tenant, err := s.tenantForHost(r.Context(), hostname(routingHost))
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				writeProblem(w, http.StatusNotFound, "no tenant is configured for this host")
				return
			}
			s.logger.Error("tenant resolution failed", "err", err, "host", routingHost)
			writeProblem(w, http.StatusInternalServerError, "internal error")
			return
		}
		ctx := withTenant(r.Context(), tenant)
		ctx = withRoutingHost(ctx, hostname(routingHost))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// isCustomDomainHost reports whether host is a bring-your-own custom domain rather
// than a tenant subdomain under the base domain — the mirror of tenantForHost's
// routing split. Owner login is confined to the main instance domain (subdomains),
// so custom domains, which are public-giver-only, gate every login affordance off
// (ADR-0008; #12). With no base domain configured (single-domain deployment) there
// is no "main domain" distinction, so nothing is treated as custom.
func (s *Server) isCustomDomainHost(host string) bool {
	if s.baseDomain == "" || host == s.baseDomain {
		return false
	}
	return !strings.HasSuffix(host, "."+s.baseDomain)
}

// hostForRouting returns the host used to resolve the tenant. By default it is the
// request Host. When forwarded-host trust is explicitly enabled (ADR-0004 §7) — i.e.
// the backend is reachable ONLY behind the trusted frontend proxy — an
// X-Forwarded-Host header takes precedence, falling back to Host when absent.
//
// SECURITY: X-Forwarded-Host is client-settable, so honoring it unconditionally
// would let anyone who can reach the backend spoof any tenant. Trust is therefore
// OFF by default and must be enabled only when the backend port is not externally
// reachable; a directly-exposed backend must keep it off.
func (s *Server) hostForRouting(r *http.Request) string {
	if s.trustForwardedHost {
		if xfh := r.Header.Get("X-Forwarded-Host"); xfh != "" {
			return xfh
		}
	}
	return r.Host
}

// tenantForHost applies the routing policy: a host under the configured base
// domain resolves by its leftmost subdomain label; anything else is treated as a
// bring-your-own custom domain. Host-string parsing lives here, not in storage.
func (s *Server) tenantForHost(ctx context.Context, host string) (storage.Tenant, error) {
	if s.baseDomain != "" && strings.HasSuffix(host, "."+s.baseDomain) {
		sub := strings.TrimSuffix(host, "."+s.baseDomain)
		return s.store.TenantBySubdomain(ctx, firstLabel(sub))
	}
	return s.store.TenantByCustomDomain(ctx, host)
}

// requireOwner enforces owner authentication on the owner surface (/api/v1/*),
// resolving the principal from a validated JWT (ADR-0005). The public surface,
// /healthz, and the unauthenticated auth endpoints (/api/v1/auth/*, e.g. login)
// pass through untouched.
//
// Two load-bearing checks: the token is validated with the algorithm pinned to
// HS256 (auth.Issuer rejects alg:none / any mismatch), and the token's tenant
// claim must equal the Host-resolved tenant — a token minted for one tenant is
// rejected on another tenant's host (no cross-tenant replay). Every issued token is
// an owner token; the instance-admin capability is a per-user flag checked only on
// the /admin surface (ADR-0010), so it grants nothing here.
func (s *Server) requireOwner(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/v1/") || strings.HasPrefix(r.URL.Path, "/api/v1/auth/") {
			next.ServeHTTP(w, r)
			return
		}
		tenant, ok := tenantFromContext(r.Context())
		if !ok {
			writeProblem(w, http.StatusUnauthorized, "authentication required")
			return
		}
		token := bearerToken(r)
		if token == "" {
			writeProblem(w, http.StatusUnauthorized, "missing bearer token")
			return
		}
		principal, err := s.auth.Issuer().Validate(token)
		if err != nil {
			writeProblem(w, http.StatusUnauthorized, "invalid or expired token")
			return
		}
		// Positive role assertion: only an owner token is accepted here. Combined with
		// the tenant-match invariant below (the token's tenant must equal the
		// Host-resolved tenant), a token minted for one tenant cannot reach another's
		// owner surface (ADR-0005 §5).
		if principal.Role != auth.RoleOwner {
			writeProblem(w, http.StatusUnauthorized, "owner authentication required")
			return
		}
		if principal.TenantID != tenant.ID {
			writeProblem(w, http.StatusUnauthorized, "token does not match this tenant")
			return
		}
		owner, err := s.store.ForTenant(tenant).Users().Get(r.Context(), principal.UserID)
		if err != nil {
			writeProblem(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		// Ban is enforced here, at the existing per-request user load (ADR-0009): a
		// banned account is rejected immediately on the owner surface — no revocation
		// store needed, and it catches an already-issued token regardless of how it
		// was minted (password or OAuth).
		if owner.Banned {
			writeProblem(w, http.StatusUnauthorized, "this account is suspended")
			return
		}
		next.ServeHTTP(w, r.WithContext(withOwner(r.Context(), owner)))
	})
}

// requireAdmin enforces the instance-admin capability on the /admin/* surface
// (ADR-0010). Admin is a per-user flag, not a separate identity: the caller holds an
// ordinary owner session (the /admin surface is not tenant-scoped, so the token
// carries its own home tenant). The middleware validates that owner token, loads the
// user from its home tenant, and requires `is_admin` — and, like the owner surface,
// rejects a banned account. The load is per-request, so revoking admin (or banning)
// takes effect immediately. An owner without the capability is a 403; an unauth or
// bad token is a 401. The surface needs no separate "enabled" gate: an instance with
// no admin simply has a surface nobody can pass.
func (s *Server) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/admin/") {
			next.ServeHTTP(w, r) // non-admin paths pass through
			return
		}
		token := bearerToken(r)
		if token == "" {
			writeProblem(w, http.StatusUnauthorized, "missing bearer token")
			return
		}
		principal, err := s.auth.Issuer().Validate(token)
		if err != nil {
			writeProblem(w, http.StatusUnauthorized, "invalid or expired token")
			return
		}
		// Load the user from the token's home tenant. /admin is not Host-scoped, so the
		// tenant comes from the token, not the request Host.
		tenant, err := s.store.TenantByID(r.Context(), principal.TenantID)
		if err != nil {
			writeProblem(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		user, err := s.store.ForTenant(tenant).Users().Get(r.Context(), principal.UserID)
		if err != nil {
			writeProblem(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		if user.Banned {
			writeProblem(w, http.StatusUnauthorized, "this account is suspended")
			return
		}
		if !user.IsAdmin {
			writeProblem(w, http.StatusForbidden, "instance-admin capability required")
			return
		}
		next.ServeHTTP(w, r.WithContext(withOwner(r.Context(), user)))
	})
}

// captureCapabilityToken lifts the X-Capability-Token header into the request
// context. The token is modeled as an apiKey security scheme in the spec, so it
// is not bound into the generated request objects; giver handlers read it from
// context (validated per-object against the stored hash).
func captureCapabilityToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if tok := strings.TrimSpace(r.Header.Get("X-Capability-Token")); tok != "" {
			r = r.WithContext(withCapToken(r.Context(), tok))
		}
		next.ServeHTTP(w, r)
	})
}

// captureClientIP lifts the request's peer IP into the context for login
// rate-limiting. It uses RemoteAddr (the direct peer); behind a reverse proxy the
// operator would front this with trusted X-Forwarded-For handling — a deployment
// concern, not done here.
func captureClientIP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(withClientIP(r.Context(), clientIP(r.RemoteAddr))))
	})
}

// clientIP strips the port from a RemoteAddr ("host:port" → "host").
func clientIP(remoteAddr string) string {
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
		return host
	}
	return remoteAddr
}

func bearerToken(r *http.Request) string {
	if rest, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
		return strings.TrimSpace(rest)
	}
	return ""
}

// hostname lowercases the Host and strips any port.
func hostname(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	return host
}

// firstLabel returns the leftmost dot-separated label.
func firstLabel(s string) string {
	if i := strings.IndexByte(s, '.'); i >= 0 {
		return s[:i]
	}
	return s
}
