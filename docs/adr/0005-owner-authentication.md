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

- **Username + password.** Passwords are stored only as a strong password-hash —
  **argon2id** (`golang.org/x/crypto/argon2`, the current OWASP recommendation for a
  new system: memory-hard, and free of bcrypt's 72-byte silent-truncation footgun).
  Use a documented preset (OWASP argon2id baseline: ~19 MiB, t=2, p=1) so it is not a
  tuning burden. The plaintext is never stored or logged.
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
- **Algorithm pinning (non-negotiable).** The verifier **pins the accepted algorithm
  to HS256** and rejects `alg: none` and any algorithm other than HS256 — it never
  trusts the token's own `alg` header to select the verification method. This closes
  the classic alg-confusion / none-algorithm JWT attack. Explicit, and pinned by a
  test.
- **Secret.** From the environment only (`YAADEGAR_AUTH_JWT_SECRET`), never a config
  file, never inlined — consistent with the project's secrets policy. A minimum
  length is enforced; unset or too-short fails closed at startup (§4).
- **Claims.** `sub` (user id), `tid` (tenant id), `role` (§7), plus `iss`, `iat`,
  `nbf`, `exp`. The middleware validates signature + `exp`/`nbf` and resolves the
  principal from `sub`/`tid`/`role`.
- **Tenant-match invariant.** For **tenant-bound principals (owners)** on the owner
  surface, the token's `tid` must equal the tenant resolved from the request Host
  (ADR-0004): a token minted for tenant A is **rejected** on tenant B's host — no
  cross-tenant replay. This stays strict on `/api/v1` and is pinned by a test. The
  instance-level superadmin is the sole carve-out — it is validated by role on a
  separate non-tenant-scoped surface, never by tenant-match (§6).
- **Lifetime (v1 = access-only).** Cut A ships a stateless **access JWT** with a
  moderate TTL (~8–12h) and re-login on expiry — no server-side session store. The
  accepted tradeoff: no server-side revocation before expiry, bounded by the TTL.
  Rotating **refresh tokens** (opaque, hashed via `internal/token`, with a
  `/auth/refresh` endpoint, a shorter access TTL, and `logout` for real revocation)
  are deferred to **Cut A′** and designed to reuse the capability-token crypto.

### 6. Role model

- **superadmin** — one instance-level administrator, authenticated via the same
  enabled login methods. Instance-scoped (tenant management / global ops), not tied to
  a single tenant.
  - **Surface + tenant-match carve-out.** Superadmin operates on a **separate,
    non-tenant-scoped admin surface** (a distinct path prefix), *not* `/api/v1`. Its
    JWT is instance-scoped — `role=superadmin`, with no tenant-bound `tid` (a
    sentinel) — and the admin surface authorizes by **role, not tenant-match**. The
    owner surface keeps the tenant-match invariant strict (§5); the carve-out is only
    for this instance-level principal. The v1 admin surface is minimal (most endpoints
    deferred), so this is one small surface + a clear rule.
  - **Bootstrap.** The superadmin is a configured **identity that logs in through the
    enabled methods** — a config-set username/email paired with a *hashed* credential,
    or an OAuth / magic-link identity. A **plaintext password is never stored in
    config**. This gives a fresh instance an administrator without an open bootstrap
    endpoint.
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

### 9. Abuse resistance (noted, not built here)

Two abuse vectors are called out for later handling (non-blocking for this ADR, likely
leaning on the captcha / a rate-limiter): **password-login brute force** (a per-account
or per-IP rate-limit or lockout) and **magic-link email-bombing** (a send rate-limit so
the login endpoint can't be used to spam an address). Each auth cut should avoid
designing these out.

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
- Deferred and referenced, not baked out: refresh tokens + logout/revocation (Cut A′),
  co-owner API (#25), captcha implementation (#45), and guest reserver tiers (#19).

### Rollout (proposed implementation cuts)

Reviewed and sized with this ADR; each cut is its own PR (2-of-2), spec-first where it
touches endpoints:

1. **Cut A — auth core + password + fail-closed startup + multi-owner model
   (access-only).** `internal/auth` (JWT issue/validate with **HS256 alg-pinning**,
   argon2id password hashing), the owner-auth middleware replacing the stub + the
   tenant-match invariant, the role/claims model, the `list_owners` join table (v1
   single-owner enforced) + the superadmin role, bootstrap & separate admin surface,
   the auth enable-flags config + JWT secret, and the at-least-one-enabled fail-closed
   startup check with **password** as the first satisfying method. Access-only session
   (moderate TTL). The foundational, MVP-unblocking cut.
2. **Cut A′ — refresh tokens + logout.** Rotating hashed refresh token, `/auth/refresh`,
   a shorter access TTL, and server-side revocation on logout.
3. **Cut B — magic-link login.** Email one-time-token → session, reusing
   `internal/token` + `internal/email`; its enable-flag + config validation.
4. **Cut C — Google OAuth login (#21).** Per-tenant OAuth2/OIDC authorization-code
   flow, ID-token verification, account link/create; its enable-flag + config
   validation.

Captcha (#45), guest tiers (#19), and co-owner API (#25) are coordinated but tracked
under their own issues, not #30 cuts.
