# ADR-0010: Admin as a per-user capability

**Status:** Accepted

**Refines** [ADR-0009](0009-identity-roles-and-registration.md) §3 (and the admin
mechanics its Cut 1 shipped). ADR-0009 remains authoritative for everything else:
the two identity axes (§1), the self-registration policy (§2), the admin
user-management surface's *feature set* (§4), the `registered` reserve tier (§5),
and the role/ban migration (§Config, Cut 1). This ADR changes only **how the
instance-admin capability is carried and authenticated** — nothing about what an
admin can do.

## Context

ADR-0009 §3 reconciled "the operator wants a per-instance admin flag" with
ADR-0005 §6's existing `superadmin` by keeping them as **one concept** but
**recommending the internal mechanics stay unchanged** — a separately-configured
identity with its own credential, on the non-tenant-scoped `/admin` surface. Cut 1
built exactly that:

- a distinct `admins` table and an `EnsureAdmin` / `AdminByUsername` / `AdminByID`
  store, separate from `users`;
- a standalone login (`POST /admin/auth/login`) issuing a JWT with
  `role=superadmin` and the sentinel tenant id `__superadmin__`;
- `requireAdmin` authorizing purely by that role, never applying the tenant-match
  invariant;
- bootstrap via the `YAADEGAR_SUPERADMIN_USERNAME` / `YAADEGAR_SUPERADMIN_PASSWORD_HASH`
  env pair, enabling the surface only when both are set;
- a separate frontend session — its own `yaadegar_admin_session` httpOnly cookie,
  its own `/admin/login` and `/admin/logout`, distinct from the owner session.

The operator has since reversed the §3 recommendation: **admin must be a capability
on an existing user account — one login that is both owner and admin — not a second
identity with its own credential and cookie.** That moots the separate admin
credential, the separate admin login, and the separate admin session cookie. This
ADR records that decision and its migration, so the mechanics land before any
rework merges.

The core tension to resolve explicitly: the `/admin` surface is deliberately **not
tenant-scoped** (it manages every tenant), yet a `users` row is **tenant-scoped**
(it carries a `tenant_id`). "Admin as a flag on a user" therefore has to say which
user carries the capability and how an account rooted in one tenant reaches an
instance-wide surface without weakening the owner/tenant boundary.

## Decision

### 1. Admin is a boolean capability on `users`

Add an `is_admin` flag to the `users` row (a new migration; see §5). The
instance-admin capability is that flag and nothing else. There is **no** separate
admin identity, no `admins` table, no `role=superadmin`, and no sentinel tenant.
The capability is orthogonal to the tenant `role` axis (ADR-0009 §1): an admin is
an ordinary `owner` whose account additionally carries `is_admin = true`. `giver`
accounts may also, in principle, carry it, but the bootstrap path (§4) grants it to
owners.

### 2. One login; the owner session authorizes `/admin`

An admin authenticates through the **ordinary owner login** and holds the ordinary
owner session — the existing `yaadegar_session` JWT, tenant-scoped to the admin's
**home tenant** (the tenant their `users` row lives in). There is no second login
and no second cookie.

`requireAdmin` is reworked to:

1. validate the owner JWT (HS256, as `requireOwner` does);
2. load the user from the token's home tenant (`principal.TenantID` +
   `principal.UserID`) — the same authoritative per-request user load `requireOwner`
   already performs;
3. require `is_admin = true` **and** `banned = false` on that freshly-loaded row.

Because the check reads the row every request, **revoking admin (or banning) takes
effect immediately** — the same property ADR-0009 §4 relies on for ban, with no
revocation store.

### 3. Instance-wide reach is a property of the surface, not the token

This is the resolution of the tenant-scoped-record / instance-wide-surface tension:

- **The token stays tenant-scoped.** On `/api/v1`, an admin is *only* an owner in
  their home tenant; the tenant-match invariant (ADR-0005 §5) is unchanged, so an
  admin cannot read or write another tenant's owner data through `/api/v1`. The
  `is_admin` flag grants **nothing** on `/api/v1`.
- **The `/admin` surface is where instance-wide reach lives.** `/admin` is already
  non-tenant-scoped by construction (`resolveTenant` skips it; it operates across
  all tenants — list tenants, create a tenant, manage any tenant's users). Access
  to that surface is gated solely by the `is_admin` load in §2. So an admin's
  cross-tenant power is confined to `/admin` and exists only because they hold the
  capability — not because their token is special.

The owner/admin boundary that ADR-0005 §6 guarded with *two* independent checks
(role assertion + tenant-match) now rests on **one** authoritative check: the
per-request `is_admin` load. That is deliberate and is the whole point of the
pivot — the capability, not a distinct token shape, is what distinguishes an admin.
An owner who is not flagged is rejected at `/admin` with 403; the same owner keeps
full ordinary access to `/api/v1` for their tenant. The check is a positive
assertion (`is_admin == true`), never an absence-of-negative.

### 4. Bootstrap and grant — no env credential

With no separate admin identity, the first admin is an **existing owner flipped to
`is_admin`**. The bootstrap credential is that owner's own (already-hashed) password
— there is no admin-specific secret, so `YAADEGAR_SUPERADMIN_USERNAME` /
`YAADEGAR_SUPERADMIN_PASSWORD_HASH` are retired.

- **Headless bootstrap (v1):** a CLI grant — extend `create-owner` with an
  `--admin` flag, or a small `grant-admin <tenant> <email>` command — sets
  `is_admin` on an owner. This composes with the existing headless
  create-tenant / create-owner bootstrap and is the replacement for the env pair.
- **The `/admin` surface is always mounted**, gated purely by the capability. The
  env-driven "enabled only when a superadmin is configured" toggle (`adminEnabled`,
  the 404-when-unconfigured behaviour) is dropped: an instance with zero admins
  simply has a surface nobody can pass (403), which is the correct closed state and
  needs no separate enable flag.
- **Recommendation — admin-grants-admin via the UI is a follow-up, not v1.** The
  flag makes "an admin promotes another user to admin" a one-field change, but v1
  keeps the grant path to the CLI to avoid widening the admin surface in the same
  cut as the rework. Flagged here so the door is explicit; the operator can pull it
  into v1 if desired.

### 5. Migration from the Cut-1 separate-identity model

Additive on the schema, subtractive on the retired identity. On the schema:

- **Add** `is_admin` to `users` (a new migration, both dialects, default `false` —
  every existing user stays a non-admin until explicitly granted).
- **The role/ban columns from ADR-0009's Cut 1 (migration 0015) STAY.** `role`
  (`owner`/`giver`) is the tenant axis (ADR-0009 §1) and `banned` is enforcement
  (§4) — both are orthogonal to the admin-identity question and are unaffected by
  this pivot. This ADR does not touch them.
- The `admins` table becomes unused. Whether to drop it in the same migration or
  leave it inert is a small implementation call for Phase 2; dropping it is cleaner
  and there is no pre-1.0 data to preserve.

Retired (removed in the Phase-2 rework):

- the `POST /admin/auth/login` endpoint and its handler; the `role=superadmin`
  value and the `__superadmin__` sentinel tenant; the `admins` store
  (`EnsureAdmin` / `AdminByUsername` / `AdminByID`);
- the `YAADEGAR_SUPERADMIN_USERNAME` / `YAADEGAR_SUPERADMIN_PASSWORD_HASH` env pair
  and the startup bootstrap that consumed them;
- on the frontend: the `yaadegar_admin_session` cookie, the `/admin/login` and
  `/admin/logout` routes, the separate admin-session helper, and the second token
  in the request locals — the admin frontend reuses the owner session.

Operator-visible upgrade step: on a running instance the previously-configured
superadmin identity disappears; the operator instead grants `is_admin` to an
existing owner via the CLI (§4). This is stated so the migration is not silent. As
a pre-1.0 project with the admin surface freshly shipped, a clean cutover (no
dual-run compatibility shim) is acceptable.

### 6. Preserved guarantees

The rework is behaviour-preserving for everything except the admin mechanics above.
The Cut-1 safety guards ADR-0009 §4 established are **retained**:

- **demotion-vs-ownership (409):** demoting an owner to giver while they own lists
  is still rejected (owner access flows through `list_owners`);
- **ban enforcement** at token issue and at the per-request user load, on both the
  owner and the admin surface (the admin load in §2 checks `banned` too);
- **no privilege or affordance leak to non-admins:** every `/admin` call is
  authoritatively gated server-side; the frontend only *reveals* admin navigation
  when the owner's `is_admin` is true (surfaced via the owner `me` payload), which
  is a display convenience, never the gate.

Unchanged and load-bearing: the two-surface boundary and JWT/capability separation
(ADR-0005 §2), the tenant-match invariant on `/api/v1` (ADR-0005 §5), reserver
anonymity (ADR-0002 §5).

## Consequences

- A migration adds `is_admin` to `users`; `requireAdmin` becomes a capability check
  on the owner principal; the separate admin login, cookie, env credential, role,
  sentinel tenant, and `admins` store are removed. The admin frontend loses its
  second login and reuses the owner session.
- The owner/admin boundary is now enforced by a single authoritative per-request
  flag load rather than by token shape. This is simpler and makes revocation
  immediate, at the cost of the old "two independent checks" redundancy — an
  intentional trade the operator has chosen.
- ADR-0005 §6's superadmin mechanics are refined away (the *concept* of a single
  instance administrator remains; its *carrier* changes from a separate identity to
  a flag on an owner). ADR-0009 §3's recommendation to keep the internal superadmin
  mechanics is reversed by this ADR; ADR-0009 §1/§2/§4-feature/§5 are unchanged.
- Bootstrap moves from an env credential to a CLI grant on an existing owner. No
  admin-specific secret exists after this change.
- Spec-first (ADR-0002): the OpenAPI contract loses `POST /admin/auth/login` and
  the admin-login response; the `me` payload gains `is_admin`. The spec change
  regenerates **both** the Go server types and the frontend client (the CI drift
  guard covers both).

## Rollout

Single implementation phase after this ADR is signed off (one PR, or a thin
backend/frontend split, 2-of-N review, spec-first):

1. Migration: add `users.is_admin`.
2. Backend: rework `requireAdmin` to the capability load; remove the admin login,
   role, sentinel, and `admins` store; add the CLI grant; expose `is_admin` on the
   owner `me` payload; retire the superadmin env config.
3. Frontend: drop the admin cookie / login / logout; gate admin navigation on the
   owner `me.is_admin`; the admin pages call `/admin/*` with the owner session.

No implementation lands before this ADR is signed off.
