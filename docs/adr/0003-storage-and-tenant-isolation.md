# ADR-0003: Storage layer and structural tenant isolation

**Status:** Accepted

## Context

[ADR-0001](0001-foundations.md) fixed pluggable storage behind a repository
interface — SQLite for dev/test, Postgres for production — and multi-tenant
Host-header routing from day one. [ADR-0002](0002-api-design.md) fixed the domain
(users/tenants, lists, items, reservations, contributions, matches, domains) and
stated that all data access is tenant-scoped **at the storage layer**, so
cross-tenant leakage is impossible by construction. This ADR records how the
storage layer is shaped and how that isolation is actually enforced, before the
handlers in #5 build on it.

## Decision

1. **One interface, two drivers, one implementation.** `internal/storage` defines
   the repository interfaces and domain types. A single `database/sql`-based
   implementation (`internal/storage/sqlstore`) backs **both** drivers; a small
   **`Dialect`** abstracts the handful of differences (parameter placeholders,
   a few DDL types). SQLite (`modernc.org/sqlite`, pure-Go — no cgo, so `-race`
   and CI stay simple and the binary stays static) and Postgres (`jackc/pgx/v5`)
   differ only by dialect, DSN, and migration set, chosen by config. One CRUD
   body is under test, not two parallel ones.

2. **Tenant isolation by construction.** The only way to reach domain data is
   `Store.ForTenant(t) → TenantStore`. A `TenantStore` holds a **private**
   `tenantID`; every repository it hands out is bound to that id, and every SQL
   statement the drivers issue takes `tenant_id` **from that bound value, never
   from a caller-supplied argument**. No repository method accepts an arbitrary
   tenant or omits the scope. The only unscoped operations are tenant creation and
   the tenant-resolution lookups (`TenantBySubdomain`, `TenantByCustomDomain`,
   `TenantByID`) — these are the *doors* to a tenant scope, not a way around it.
   Host-string parsing (which lookup to use) stays in the caller; storage holds no
   base-domain policy. Every table carries a `tenant_id` column.

   **Boundary (stated honestly):** this is enforced by the repository package
   being the sole data path, plus review and tests — not by the compiler. The
   invariant is simple and checkable: *no SQL in `sqlstore` omits `tenant_id`*,
   and a cross-tenant isolation test asserts that one tenant's handle can never
   read or mutate another tenant's rows. Any new repository method must uphold it.

3. **Capability tokens are stored hashed.** Reservations and contributions are
   looked up by a token **hash** (`ByTokenHash`); the storage layer never sees or
   persists the raw one-time token (ADR-0002 §4). Generating and hashing the token
   is the caller's (service) job — storage stays crypto-free — so a database
   compromise never yields usable tokens.

4. **Migrations: embedded, per-dialect, versioned.** SQL lives in
   `migrations/sqlite/` and `migrations/postgres/`, embedded via `embed.FS`. A
   runner applies files in version order inside a transaction each and records
   applied versions in a `schema_migrations` table. Forward-only for now.

5. **Portable types.** IDs and share slugs are opaque `TEXT` (UUIDs generated in
   Go, so no dialect-specific autoincrement); timestamps are RFC 3339 UTC `TEXT`;
   money is an integer minor-unit amount + `TEXT` ISO-4217 currency (ADR-0002 §10);
   booleans are `INTEGER` 0/1. DDL stays near-identical across the two dialects and
   money arithmetic stays exact.

## Consequences

- Adding a third database later is a new `Dialect` + migration set, not a new
  repository implementation.
- The tenant-bound handle makes the classic multi-tenant bug — forgetting a
  `WHERE tenant_id = …` — structurally hard: there is no unscoped repository to
  call in the first place.
- Deriving an item's `availability` / `amount_funded` combines reservations,
  contributions, and matches; that is business logic and lands with the handlers
  in #5. Storage persists and returns the raw entities plus a few aggregate
  helpers (reserved quantity, funded amount) so #5 avoids N+1 reads.
- Storing only token hashes means a leaked database does not expose usable
  capability tokens.
