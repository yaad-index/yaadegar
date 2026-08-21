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
  user-management surface; refines ADR-0005 §6 and ADR-0008 §5. **Status: Accepted.**
- [ADR-0010: Admin as a per-user capability](0010-admin-as-a-user-capability.md) —
  the instance-admin capability becomes an `is_admin` flag on an existing owner
  account (one login is both owner and admin) instead of a separate admin identity;
  retires the separate admin login, session cookie, and superadmin env credential;
  `requireAdmin` becomes a per-request capability load with tenant-scoped tokens and
  instance-wide reach confined to `/admin`; refines ADR-0009 §3. **Status: Proposed.**
- [ADR-0011: Password lifecycle](0011-password-lifecycle.md) — a per-user
  `credential_version` claim checked in `requireOwner` so every password mutation
  immediately revokes prior sessions (an eyes-open trade against ADR-0005 §1's
  stateless validation — the check folds into the user load the middleware already
  does, and ships without a cache; an in-process cache is a documented future
  optimization); an authenticated change-password endpoint that re-issues only the
  acting session; an enumeration-safe email forgot-password reset that auto-logs-in
  on confirm; and one version-bumping mutation funnel for all four password entry
  points (set-password CLI, change, reset, create-owner); addresses #142 + #148.
  **Status: Accepted.**
- [ADR-0012: Registration mechanics](0012-registration-mechanics.md) — fills in
  ADR-0009's deferred self-registration and registered-giver-reserve cuts without
  changing its policy/role model: two policy-gated signup methods (Google OAuth
  one-click no-password vs email+password with CAPTCHA + email-link verification),
  the empty-hash "no password set" state reused from ADR-0011 (change-password 403s,
  reset flow establishes a password), a single-use email-verification token store
  mirroring `password_reset_tokens`, one unified invite/set-password onboarding flow
  for admin-created and no-password accounts (rejecting emailed temp-passwords), and
  the authenticated `registered`-tier reserve with a reserver dashboard and no
  per-reservation email (anonymity to the owner preserved). **Status: Accepted.**
- [ADR-0013: Anti-bot CAPTCHA on low-trust reserve](0013-anti-bot-captcha.md) — a
  pluggable `captcha.Verifier` (nil-default `NoopVerifier`, so disabled instances are
  unchanged) with managed-provider impls (Turnstile default, hCaptcha, reCAPTCHA)
  gating the two low-trust reserve tiers server-side before create; an optional
  `captcha_token` on the reserve request, absent-token → 400 before `Verify`;
  operator-supplied env config that fails closed on an unknown provider; a
  frontend widget shown only when enabled and the tier is low-trust; and a bounded
  fail-closed 5s verify timeout. Fills the `captchaGate` seam, depends on ADR-0007,
  closes #45's design (Altcha PoW deferred to a follow-up cut). **Status: Accepted.**
- [ADR-0014: Publishing the web image](0014-publish-web-image.md) — publish a second
  image (`yaadegar-web`) from the `web/` context instead of combining the two, keeping
  the API's distroless single-binary runtime and the unambiguous `exec app yaadegar`
  seeding path; both images built and pushed in one workflow run from one metadata step
  so a matched pair is the default, with the #190/#193 stamp guards extended to the web
  image and neither pushed until both verify; a new unauthenticated `/version` route
  (deliberately not a field on the `text/plain` `/healthz`, which would be a content-type
  break for operator probes) that the web service reads at startup so skew is loud rather
  than silent and externally pollable afterwards, logging rather than refusing to start;
  and a docs compose that CI stands up and asserts serves a site, with the backend port
  left unpublished because ADR-0004 §7's forwarded-host trust depends on it. Also keeps an
  API-only deployment expressible. Closes #258's design, unblocks #236.
  **Status: Proposed.**
