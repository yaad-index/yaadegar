package api

import (
	"context"

	"github.com/yaad-index/yaadegar/internal/storage"
)

// ownerRoleRequiredDetail is the single detail returned by every owner-role gate, so
// the response reads consistently across the owner-only endpoints.
const ownerRoleRequiredDetail = "this action requires an owner account"

// hasOwnerRole reports whether the request's authenticated account holds the
// per-tenant owner role (ADR-0009). requireOwner already loaded the account into
// context and asserted the JWT role; this is the second, orthogonal check on the
// stored tenant role (owner vs giver), so a giver's valid session is refused the
// owner-only surface. It reads from context — no extra DB load. A missing owner in
// context (should not happen behind requireOwner) reports false, failing closed.
func hasOwnerRole(ctx context.Context) bool {
	owner, ok := ownerFromContext(ctx)
	if !ok {
		return false
	}
	return owner.Role == storage.RoleOwner
}
