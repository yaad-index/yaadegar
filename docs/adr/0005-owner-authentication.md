# ADR-0005: Owner authentication

**Status:** Proposed

## Context

[ADR-0002](0002-api-design.md) split the service into two surfaces — an owner
surface (`/api/v1`, everything an account holder does) and a public giver surface
(share links, reserve, contribute) — and deferred the real owner login/session
mechanism (§4), shipping instead a **documented, not-secure bearer stub**: the
middleware trusts the bearer token as the owner's user id and only checks it
against the tenant. That stub grants access to anyone who can guess a user id and
must not survive to the first release (#30). This ADR fixes the real mechanism.

The public surface is out of scope here: it stays anonymous via the one-time,
hashed **capability-token** scheme (ADR-0002 §5, ADR-0003 §3) and does not change.
This ADR governs only *who is an authenticated principal* and *how they prove it*.

The design is self-hosted-first: the instance operator, not Yaadegar, decides which
login methods exist, so the mechanism must be configurable and refuse to run in an
un-authenticatable state rather than fall open.

## Decision

### 1. Mechanism — signed JWT as a Bearer token

A successful login issues a signed **JSON Web Token** presented as
`Authorization: Bearer <jwt>`. An owner-auth middleware validates it on every
owner-surface request and resolves the principal (user + tenant + role), replacing
the stub. **Nothing is open by default** — the owner surface requires a valid token
for every non-login endpoint. Validation is stateless: the middleware verifies the
signature and standard claims without a database round-trip for the access token
itself.

### 2. Surface boundary — an invariant

Two surfaces, two independent auth schemes, no crossover:

- **Owner surface (`/api/v1`)** — JWT Bearer, this ADR.
- **Public surface** — anonymous capability tokens, unchanged (ADR-0002 §5).

A JWT is **never** required or accepted on the public surface, and a capability
token is never accepted as owner auth. Guests are not JWT principals in v1 (see
§7). This separation is load-bearing and is pinned by a test.

### 3. The three methods converge on one session issuer

Three login methods are supported, each **independently enable-able by the
operator**: username+password, Google OAuth, and magic-link (email). They differ
only in how they *authenticate a user*; they all end at a single
`internal/auth` **session issuer** that, given an authenticated `User` + tenant +
role, mints the session token(s). Adding or removing a method never touches token
issuance or the middleware.

- **Username + password.** Passwords are stored only as a strong password-hash
  (recommended: **bcrypt** via `golang.org/x/crypto/bcrypt` — mature and hard to
  misuse; **argon2id** is the stronger alternative, noted for the operator/reviewers
  to choose). The plaintext is never stored or logged.
- **Magic-link.** A login request emails a **one-time, hashed, short-TTL** token
  link, reusing `internal/token` (raw emailed once, only the SHA-256 hash stored —
  exactly the capability-token pattern) and `internal/email.Sender` (#37). Clicking
  the link verifies the token and issues the session. Requires a configured email
  sender and link base.
- **Google OAuth.** Standard OAuth2 / OIDC authorization-code flow with **per-tenant
  client credentials**, folding in #21. The returned ID token is verified, its email
  is mapped to a `User` (create-or-link within the tenant), and the session is
  issued. This is the largest method and lands last.

### 4. Enable-flags and the fail-closed startup invariant

Each method has an operator config flag plus its required settings (password: on/off;
OAuth: client id/secret/redirect; magic-link: on/off + a working email sender +
link base). At startup the server:

1. Collects the set of **enabled** methods.
2. Validates that **every enabled method is correctly configured**, naming the method
   and the missing/invalid field on failure.
3. Requires the **JWT signing secret** to be present and strong (see §5).

**Hard invariant:** if **zero** methods are enabled — or an enabled method is
misconfigured, or the signing secret is missing/weak — the instance **refuses to
start** with a clear, actionable error. Two or three enabled is fine; one is the
floor. There is no silent open default. This startup validation is a load-bearing
test.

### 5. JWT mechanics

- **Signing.** Symmetric **HMAC-SHA256 (HS256)** with a server-side secret. The same
  single-binary process signs and verifies, so a symmetric secret is sufficient and
  simplest; asymmetric (EdDSA/RS256) is noted as a future option only if external
  verification is ever needed.
- **Secret.** From the environment only (`YAADEGAR_AUTH_JWT_SECRET`), never a config
  file, never inlined — consistent with the project's secrets policy. A minimum
  length is enforced; unset or too-short fails closed at startup (§4).
- **Claims.** `sub` (user id), `tid` (tenant id), `role` (§7), plus `iss`, `iat`,
  `nbf`, `exp`. The middleware validates signature + `exp`/`nbf` and resolves the
  principal from `sub`/`tid`/`role`.
- **Tenant-match invariant.** The token's `tid` must equal the tenant resolved from
  the request Host (ADR-0004). A token minted for tenant A is **rejected** on tenant
  B's host — no cross-tenant replay. Pinned by a test.
- **Lifetime + refresh.** A short-lived **access JWT** (recommended ~1h) plus an
  opaque, **hashed, rotating refresh token** stored server-side via `internal/token`
  (same crypto as capability tokens). A `/auth/refresh` endpoint trades a valid
  refresh token for a new access JWT and a rotated refresh token; **logout** deletes
  the refresh token, giving real revocation while keeping request-time validation
  stateless. (If the reviewers prefer a smaller first cut, an access-only variant
  with a moderate TTL and re-login on expiry is a viable v1 fallback — refresh can be
  its own cut; see Rollout.)

### 6. Role model

- **superadmin** — one instance-level administrator, authenticated via the same
  methods. Instance-scoped (tenant management / global ops), not tied to a single
  tenant's list surface. Bootstrapped from configuration (an operator-set superadmin
  identity), so a fresh instance has an administrator without an open endpoint. Its
  admin surface is minimal for v1; most superadmin endpoints are deferred, but the
  role and its issuance land now.
- **list owner** — an authenticated account holder who owns lists within a tenant.
- **guest** — the public reserver, tiered `full_guest` / `email_confirmed` /
  `registered` per the per-list reserver-identity policy (#19). Guests are not JWT
  principals in v1 (they act through capability tokens); a future `registered`
  reserver that carries an account would issue a JWT through the same issuer.

### 7. Data model — lists are multi-owner-capable from the start

Today a list has a single `owner_id` column. To honor "the model must allow more
than one owner per list" without a later rewrite, ownership moves to a
**`list_owners` join table** (`list_id`, `user_id`, `role`, `created_at`). **v1
enforces exactly one owner** (the creator) at the service layer, but the schema
already supports many, so collaborative co-ownership (#25) adds the API later with
no migration rewrite. This is a storage migration + repository change delivered as
part of this work.

### 8. Boundary with the anti-bot captcha (#45)

The two low-trust guest reserve paths (`full_guest` = no identity, `email_confirmed`
= email-only) additionally require the anti-bot captcha (#45) before a reservation is
created. Captcha is **orthogonal to owner auth**: it gates the public reserve path by
reserver tier (#19), and any authenticated principal is exempt. It carries no JWT and
changes nothing on the owner surface. #45 implements it; this ADR only fixes the
boundary.

## Consequences

- The stub middleware and its "bearer = user id" trust are removed before MVP; the
  owner surface is genuinely authenticated.
- Login, refresh, and magic-link-verify are **new unauthenticated endpoints**
  (`security: []`, tenant-scoped by Host) added to `api/openapi.yaml` — the spec stays
  the source of truth (ADR-0002). The `ownerBearer` scheme stays; it now denotes a
  real JWT.
- A new `internal/auth` package owns issuance, validation, password hashing, and the
  method authenticators; the config gains auth enable-flags + the JWT secret; the
  server gains fail-closed startup validation.
- Operators must configure at least one method (and the JWT secret) or the instance
  will not start — an intentional, documented behavior change from the stub.
- Deferred and referenced, not baked out: co-owner API (#25), captcha implementation
  (#45), guest reserver tiers (#19), and — if the reviewers choose — refresh tokens as
  a distinct cut.

### Rollout (proposed implementation cuts)

Reviewed and sized with this ADR; each cut is its own PR (2-of-2), spec-first where it
touches endpoints:

1. **Cut A — auth core + password + fail-closed startup + multi-owner model.**
   `internal/auth` (JWT issue/validate, password hashing), the owner-auth middleware
   replacing the stub + the tenant-match invariant, the role/claims model, the
   `list_owners` join table (v1 single-owner enforced) + superadmin role & bootstrap,
   the auth enable-flags config + JWT secret, and the at-least-one-enabled fail-closed
   startup check with **password** as the first satisfying method. The foundational,
   MVP-unblocking cut. (Refresh/logout can ride here or split to Cut A′ if A is too
   large.)
2. **Cut B — magic-link login.** Email one-time-token → session, reusing
   `internal/token` + `internal/email`; its enable-flag + config validation.
3. **Cut C — Google OAuth login (#21).** Per-tenant OAuth2/OIDC authorization-code
   flow, ID-token verification, account link/create; its enable-flag + config
   validation.

Captcha (#45), guest tiers (#19), and co-owner API (#25) are coordinated but tracked
under their own issues, not #30 cuts.
