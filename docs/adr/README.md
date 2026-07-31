# Architecture Decision Records

This directory holds the Architecture Decision Records (ADRs) for Yaadegar. ADRs
are the backbone of the project: significant architectural choices are captured
here first, then the code is built against them.

Each ADR is a short markdown file, numbered sequentially (`NNNN-title.md`), with a
lightweight structure: **Context**, **Decision**, **Status**, and **Consequences**.
An ADR is immutable once accepted; to change a decision, add a new ADR that
supersedes the old one (and mark the old one `Superseded by ADR-XXXX`).

## Index

- [ADR-0001: Foundations](0001-foundations.md) — API-first, Go backend, pluggable
  storage, multi-tenant routing, MIT/open-source. **Status: Accepted.**
- [ADR-0002: API design](0002-api-design.md) — two surfaces (owner `/api/v1`,
  public `/public`), Host-header tenancy, bearer + capability-token auth,
  reservation anonymity, opt-in co-buying handshake, RFC 9457 errors. Contract in
  [`api/openapi.yaml`](../../api/openapi.yaml). **Status: Accepted.**
- [ADR-0003: Storage layer and structural tenant isolation](0003-storage-and-tenant-isolation.md)
  — one repository interface over two drivers (SQLite dev/test, Postgres prod)
  sharing a `database/sql` body via a dialect shim; tenant isolation by
  construction (`Store.ForTenant`); hashed capability tokens; embedded per-dialect
  migrations. **Status: Accepted.**
- [ADR-0004: Multi-tenant Host routing and custom domains](0004-multi-tenant-routing-and-domains.md)
  — subdomain + custom-domain routing, DNS TXT-token verification, the
  verified-only-resolves security invariant, first-add-wins hostname uniqueness,
  config-driven reserved subdomains, TLS deferred. **Status: Accepted.**
- [ADR-0005: Owner authentication](0005-owner-authentication.md) — JWT Bearer on the
  owner surface (public capability-token surface unchanged), three operator-configurable
  login methods (password, Google OAuth, magic-link) converging on one session issuer,
  fail-closed at-least-one-method startup invariant, superadmin/owner/guest roles,
  multi-owner-capable lists. **Status: Proposed.**
- [ADR-0006: Frontend architecture](0006-frontend-architecture.md) — SvelteKit + TS
  under `web/`, SSR public share pages, one route-segmented app (owner + public giver),
  TypeScript API client generated from the spec with a CI drift-guard, use-mature-libraries
  principle (component lib / superforms+zod / TanStack Query / httpOnly-cookie sessions),
  node-adapter container. **Status: Proposed.**
- [ADR-0007: Reserver-identity policy](0007-reserver-identity-policy.md) — the three
  reserver tiers (full_guest / email_confirmed / registered), per-list override, and
  the email-confirmation flow (#19). **Status: Proposed.**
- [ADR-0008: Owner login via Google OAuth / OIDC](0008-owner-oauth.md) — OIDC
  auth-code + PKCE, one fixed redirect_uri with tenant-in-state and a one-time-ticket
  cross-host session handoff, link-only-by-verified-email account model, per-tenant
  toggle over one instance-level client (refines ADR-0005 §3), env config (#21).
  **Status: Proposed.**
- [ADR-0009: Unified identity, roles, and registration](0009-identity-roles-and-registration.md)
  — two orthogonal axes (per-tenant owner/giver role + an instance-admin capability),
  first-class giver accounts that make the `registered` reserve tier real (alongside the
  untouched anonymous tiers), an instance self-registration policy, and an admin
  user-management surface; refines ADR-0005 §6 and ADR-0008 §5. **Status: Proposed.**
