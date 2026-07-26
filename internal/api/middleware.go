package api

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/yaad-index/yaadegar/internal/storage"
)

// resolveTenant resolves the tenant from the request Host and puts it in the
// context for every request except /healthz (which is host-agnostic ops). An
// unknown host is a 404 — the request names no tenant we serve.
func (s *Server) resolveTenant(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
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

// requireOwner enforces owner authentication on the owner surface (/api/v1/*).
// The public surface and /healthz pass through untouched.
//
// STUB — NOT SECURE. Until the owner-auth ADR lands, the bearer token is treated
// as the owner's user id and merely validated against the tenant. User ids are
// not secrets, so this grants access to anyone who knows one; it exists only so
// owner-scoped endpoints have a concrete principal. A real token→identity
// mechanism replaces this before MVP (tracked separately).
func (s *Server) requireOwner(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/v1/") {
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
		owner, err := s.store.ForTenant(tenant).Users().Get(r.Context(), token)
		if err != nil {
			writeProblem(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		next.ServeHTTP(w, r.WithContext(withOwner(r.Context(), owner)))
	})
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
