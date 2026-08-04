package api

import (
	"context"

	"github.com/yaad-index/yaadegar/internal/auth"
	"github.com/yaad-index/yaadegar/internal/storage"
)

// sessionRole maps a stored tenant role to the role stamped on a session token
// (#163), so the JWT reflects the account's real role instead of a hardcoded owner.
// It is informational only — authorization reads the stored role (see hasOwnerRole),
// never this claim — so an unknown role falls closed to the least-privileged giver.
func sessionRole(r storage.UserRole) auth.Role {
	if r == storage.RoleOwner {
		return auth.RoleOwner
	}
	return auth.RoleGiver
}

// ownerRoleRequiredDetail is the single detail returned by every owner-role gate, so
// the response reads consistently across the owner-only endpoints.
const ownerRoleRequiredDetail = "this action requires an owner account"

// hasOwnerRole reports whether the request's authenticated account holds the
// per-tenant owner role (ADR-0009). requireOwner already loaded the account from
// storage into context and admitted the session (owner or giver); this is the
// authorization check on the STORED tenant role — the authority, never the token
// claim (#163) — so a giver's valid session, even one carrying a stale owner claim,
// is refused the owner-only surface. It reads from context — no extra DB load. A
// missing owner in context (should not happen behind requireOwner) fails closed.
func hasOwnerRole(ctx context.Context) bool {
	owner, ok := ownerFromContext(ctx)
	if !ok {
		return false
	}
	return owner.Role == storage.RoleOwner
}
