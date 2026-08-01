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
	clientIPCtxKey
	routingHostCtxKey
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

// withRoutingHost carries the host the tenant was resolved from (lowercased, no
// port). The strict handlers have no *http.Request, so an endpoint that must
// distinguish a subdomain host from a custom domain reads it from here.
func withRoutingHost(ctx context.Context, host string) context.Context {
	return context.WithValue(ctx, routingHostCtxKey, host)
}

// routingHostFromContext returns the tenant-resolution host, or "" if unknown.
func routingHostFromContext(ctx context.Context) string {
	h, _ := ctx.Value(routingHostCtxKey).(string)
	return h
}
