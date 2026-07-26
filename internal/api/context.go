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
