package api

import (
	"context"

	"github.com/yaad-index/yaadegar/internal/api/gen"
)

// GetOwnerKey returns the signed-in owner's key for their public list index (#308),
// or null when they have never created one.
//
// This read never mints. An account has no owner page until its owner explicitly
// asks for one, so "no key yet" is the ordinary state rather than something to
// repair on the way past — minting here would give a public surface to every
// account that so much as opened the settings screen.
func (s *Server) GetOwnerKey(ctx context.Context, _ gen.GetOwnerKeyRequestObject) (gen.GetOwnerKeyResponseObject, error) {
	owner, ok := ownerFromContext(ctx)
	if !ok {
		return nil, errMissingContext
	}
	return gen.GetOwnerKey200JSONResponse(toGenOwnerKey(owner.OwnerKey)), nil
}

// CreateOwnerKey mints a fresh owner key and returns it (#308). Creating the first
// key and rotating an existing one are one operation: both replace whatever is
// stored. That is what makes rotation a revocation — the previous key stops
// resolving as soon as this returns, so an owner who shared a link too widely can
// retire it without touching any of their lists.
func (s *Server) CreateOwnerKey(ctx context.Context, _ gen.CreateOwnerKeyRequestObject) (gen.CreateOwnerKeyResponseObject, error) {
	owner, ok := ownerFromContext(ctx)
	ts, _, ok2 := s.tenantStore(ctx)
	if !ok || !ok2 {
		return nil, errMissingContext
	}
	key, err := ts.Users().RotateOwnerKey(ctx, owner.ID)
	if err != nil {
		return nil, err
	}
	return gen.CreateOwnerKey200JSONResponse(toGenOwnerKey(key)), nil
}

// toGenOwnerKey maps the stored key to the wire shape, where "" — the not-yet-minted
// sentinel — becomes an explicit null rather than an empty string, so a client can
// tell "no owner page" from a key it failed to read.
func toGenOwnerKey(key string) gen.OwnerKey {
	if key == "" {
		return gen.OwnerKey{}
	}
	return gen.OwnerKey{OwnerKey: ptr(key)}
}
