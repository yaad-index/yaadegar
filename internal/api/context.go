// Package api implements the generated StrictServerInterface (internal/api/gen)
// against the storage layer. It wires tenant resolution and owner auth as
// middleware, maps storage sentinels to RFC 9457 problem+json, and derives item
// availability from storage aggregates. Reserver/contributor identity is never
// placed on a response — the generated owner/public types carry no such field,
// so identity-hiding is structural, not a runtime filter (ADR-0002 §5).
package api

import (
	"context"

	"github.com/yaad-index/yaadegar/internal/storage"
)

type ctxKey int

const (
	tenantCtxKey ctxKey = iota
	ownerCtxKey
	capTokenCtxKey
	adminCtxKey
	clientIPCtxKey
)

// withTenant returns ctx carrying the resolved tenant for the request.
func withTenant(ctx context.Context, t storage.Tenant) context.Context {
	return context.WithValue(ctx, tenantCtxKey, t)
}

// tenantFromContext returns the tenant resolved by the tenant middleware.
func tenantFromContext(ctx context.Context) (storage.Tenant, bool) {
	t, ok := ctx.Value(tenantCtxKey).(storage.Tenant)
	return t, ok
}

// withOwner returns ctx carrying the authenticated owner for the request.
func withOwner(ctx context.Context, u storage.User) context.Context {
	return context.WithValue(ctx, ownerCtxKey, u)
}

// ownerFromContext returns the owner authenticated by the owner-auth middleware.
func ownerFromContext(ctx context.Context) (storage.User, bool) {
	u, ok := ctx.Value(ownerCtxKey).(storage.User)
	return u, ok
}

// withAdmin returns ctx carrying the authenticated superadmin for the request.
func withAdmin(ctx context.Context, a storage.Admin) context.Context {
	return context.WithValue(ctx, adminCtxKey, a)
}

// adminFromContext returns the superadmin authenticated by the admin-auth middleware.
func adminFromContext(ctx context.Context) (storage.Admin, bool) {
	a, ok := ctx.Value(adminCtxKey).(storage.Admin)
	return a, ok
}

// withCapToken carries the raw capability token from the X-Capability-Token
// header (the token is modeled as an apiKey security scheme, so it is not bound
// into the generated request objects — the middleware stashes it here instead).
func withCapToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, capTokenCtxKey, token)
}

// capTokenFromContext returns the raw capability token, if the request carried one.
func capTokenFromContext(ctx context.Context) string {
	tok, _ := ctx.Value(capTokenCtxKey).(string)
	return tok
}

// withClientIP carries the request's client IP (for login rate-limiting; the
// strict handlers have no direct access to the *http.Request).
func withClientIP(ctx context.Context, ip string) context.Context {
	return context.WithValue(ctx, clientIPCtxKey, ip)
}

// clientIPFromContext returns the request's client IP, or "" if unknown.
func clientIPFromContext(ctx context.Context) string {
	ip, _ := ctx.Value(clientIPCtxKey).(string)
	return ip
}
