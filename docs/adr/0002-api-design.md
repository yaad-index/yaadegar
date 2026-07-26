# ADR-0002: API design

**Status:** Accepted

## Context

Per [ADR-0001](0001-foundations.md) Yaadegar is API-first: the external HTTP API
is the core, and every client (web, future mobile, automation) goes through it.
Before writing handlers or storage, the API surface is designed up front so the
backend and clients build against one contract. This ADR records the cross-cutting
API decisions; the concrete contract lives in [`api/openapi.yaml`](../../api/openapi.yaml).

The domain has three distinct actors, which shapes the surface:

- **Owner** — a signed-in user who creates and manages their lists.
- **Giver** — a (usually anonymous) visitor who views a public list via its share
  link and may reserve or co-buy an item. Givers do not have accounts.
- **Operator/tenant** — a user who owns a tenant (subdomain and/or custom domains).

## Decision

1. **Two surfaces, one API.**
   - **`/api/v1/*` — owner surface**, authenticated. Manage lists, items, domains,
     account.
   - **`/public/*` — giver surface**, mostly unauthenticated, reached via a list's
     opaque **`share_slug`**. This is where reservations and co-buying happen. It
     never exposes owner-only data (draft lists, other givers' identities).

2. **Multi-tenant by Host header (ADR-0001).** The tenant is resolved from the
   request Host before routing; every request operates within exactly one tenant,
   and all data access is tenant-scoped **at the storage layer** (structural
   isolation — see the note carried from ADR-0001's review). Tenancy is implicit
   in the host, not a path parameter, so the same paths serve every tenant.

3. **Versioning.** URL-path versioning (`/api/v1`). The public surface is
   unversioned-by-path but evolves compatibly; breaking changes there would move to
   `/public/v2` if ever needed.

4. **Auth.**
   - Owner surface: **bearer token** (`Authorization: Bearer …`). The login/session
     mechanism (password, OAuth, magic-link) is deferred to a later ADR; this ADR
     only fixes that the owner surface is bearer-authenticated.
   - Giver actions that create state (a reservation, a contribution) return a
     **capability token** scoped to that object, so the anonymous giver can later
     manage it (release a reservation, confirm a match) without an account. The
     token is returned once, on creation, and required for subsequent mutation.

5. **Reservation anonymity.** A public list shows an item's *availability*
   (available / reserved) but **never who reserved it** — not to other givers, and
   by default not to the owner either (so the surprise holds). Reservation identity
   is only ever used server-side (e.g. for the decay emails).

6. **Group co-buying is an explicit, opt-in, two-sided handshake.** A contribution
   carries a pledged share plus a contact the giver agrees to share **only** if a
   match forms. When pledges cover an item, the server proposes a **match**; each
   party confirms via their capability token; only after **both** confirm are the
   contacts revealed to each other and the item marked as being co-bought. No
   contact leaks before mutual confirmation.

7. **Error shape: RFC 9457 (problem+json).** All errors return
   `application/problem+json` with `type`, `title`, `status`, `detail`. This keeps
   error handling uniform across clients.

8. **Pagination.** Collections use `limit` + `offset` query params and return a
   small envelope `{ "items": [...], "total": N, "limit": L, "offset": O }`.
   Simple and adequate for personal-scale lists; can move to cursors later if a
   collection ever grows unbounded.

9. **IDs.** Opaque server-generated string IDs (UUIDs). Share slugs are separate
   opaque, unguessable strings so a list URL can be shared without leaking the ID
   space.

10. **Timestamps + money.** Timestamps are RFC 3339 UTC strings. Money is an
    integer **minor-unit amount + ISO-4217 currency code** (never a float), so
    co-buying arithmetic is exact.

## Consequences

- Handlers split cleanly into an authenticated owner mux and a public giver mux;
  the storage layer enforces tenant scoping regardless of surface.
- The capability-token pattern lets givers act without accounts while keeping each
  action bound to its object — but tokens must be treated as secrets (returned
  once, unguessable, revocable by deleting the underlying object).
- Deferring the login mechanism keeps this ADR focused on shape; a follow-up ADR
  fixes owner authentication before the owner surface is implemented.
- The OpenAPI file is the source of truth for the contract; server and client code
  are generated from / validated against it in later issues.
