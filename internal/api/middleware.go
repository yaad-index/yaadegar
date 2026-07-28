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
		// /healthz is host-agnostic ops; /admin is the instance-level superadmin
		// surface and is deliberately NOT tenant-scoped (ADR-0005 §6).
		if r.URL.Path == "/healthz" || strings.HasPrefix(r.URL.Path, "/admin/") {
			next.ServeHTTP(w, r)
			return
		}
		tenant, err := s.tenantForHost(r.Context(), hostname(r.Host))
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				writeProblem(w, http.StatusNotFound, "no tenant is configured for this host")
				return
			}
			s.logger.Error("tenant resolution failed", "err", err, "host", r.Host)
			writeProblem(w, http.StatusInternalServerError, "internal error")
			return
		}
		next.ServeHTTP(w, r.WithContext(withTenant(r.Context(), tenant)))
	})
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
// rejected on another tenant's host (no cross-tenant replay). In Cut A1 every
// issued token is an owner token; the superadmin admin-surface carve-out lands
// with A2.
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
		// Two independent checks guard the owner/admin boundary (ADR-0005 §6). (1)
		// Positive role assertion: only an owner token is accepted here, so a
		// superadmin token is rejected on its role alone. (2) Tenant-match invariant
		// (below): the token's tenant must equal the Host-resolved tenant, which the
		// superadmin sentinel tid also fails. Either check alone closes the boundary.
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
		next.ServeHTTP(w, r.WithContext(withOwner(r.Context(), owner)))
	})
}

// requireAdmin enforces superadmin authentication on the instance-level admin
// surface (/admin/*), except the unauthenticated login endpoint. It authorizes by
// ROLE (superadmin) and never applies the tenant-match invariant — the superadmin
// is not tenant-scoped (ADR-0005 §6). This is the mirror of requireOwner's role
// check: an owner token is rejected here by role, and a superadmin token is
// rejected on the owner surface by role, so the boundary holds in both directions
// independently of the sentinel tid. When no superadmin is configured the surface
// reports 404 rather than failing the process.
func (s *Server) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/admin/") || r.URL.Path == "/admin/auth/login" {
			next.ServeHTTP(w, r) // non-admin paths and the unauth login pass through
			return
		}
		if !s.adminEnabled {
			writeProblem(w, http.StatusNotFound, "the admin surface is not enabled on this instance")
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
		if principal.Role != auth.RoleSuperadmin {
			writeProblem(w, http.StatusForbidden, "superadmin role required")
			return
		}
		admin, err := s.store.AdminByID(r.Context(), principal.UserID)
		if err != nil {
			writeProblem(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		next.ServeHTTP(w, r.WithContext(withAdmin(r.Context(), admin)))
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
