# ADR-0011: Password lifecycle (change, reset, session invalidation)

**Status:** Accepted

**Extends** [ADR-0005](0005-owner-authentication.md) (owner authentication: the
session JWT, its claims, and the `requireOwner` middleware) and builds on
[ADR-0009](0009-identity-roles-and-registration.md) / [ADR-0010](0010-admin-as-a-user-capability.md)
(the `users` row as the single identity, admin as a per-user capability).
Supersedes nothing. Addresses #142 (forgot-password reset) and #148 (session
invalidation on password change), and adds an authenticated change-password flow
that neither issue tracked on its own.

## Context

Password login shipped in ADR-0005: an owner authenticates with a username +
password, and the server issues a stateless access JWT. Per ADR-0005 §1 the token
is validated **without a database round-trip** — the middleware checks only the
HS256 signature and the standard claims (`sub`, `tid`, `role`, `iss`, `iat`, `exp`)
— and per §5 the access token is short-lived (default 12h, re-login on expiry;
refresh tokens are a later cut).

That leaves three gaps in the password lifecycle:

- **No self-service change.** A logged-in owner cannot change their own password;
  the only ways to set a password today are the `create-owner` and `set-password`
  (#141) CLI commands, both operator-side.
- **No reset.** An owner who forgets their password has no recovery path except an
  operator running `set-password`. #142 asks for an email-based self-service reset.
- **No invalidation.** Because validation is stateless, a password change or an
  operator reset **cannot revoke already-issued tokens** — a stolen or stale token
  keeps working until it expires (up to the full access TTL). #148 raised this as
  the missing half of a credible reset/breach-response story: the `set-password`
  recovery path can change the secret but can't actually lock an attacker out.

The `users` table already carries the password credential (`username`,
`password_hash`, ADR-0005 §5 / migration `0005_user_credentials`), and the codebase
already has the two primitives a reset needs: `internal/token` (raw/hashed
single-use tokens — the capability-token pattern behind reservation confirm and the
co-buy match-action token, migration `0010`) and `internal/email` (the `Sender`
interface, log/SMTP/fake). No password-strength policy is enforced anywhere yet;
each entry point hashes whatever it is given.

This ADR records the design for closing all three gaps as one coherent feature. It
is design-only; no implementation lands until it is approved.

## Decision

### 1. Credential version — the invalidation foundation

Add a per-user **`credential_version`** column (integer, `NOT NULL DEFAULT 1`) to
`users`. The issued session JWT carries it as a new claim (`cver`). The
`requireOwner` middleware, after it validates the signature and standard claims
(ADR-0005 §1), additionally **reads the user's stored `credential_version` and
rejects the token if the claim does not match**. Every password mutation
increments the stored version, so at the instant a password changes, **all
previously-issued tokens for that user become invalid** — they carry the old
version and fail the check.

This is a deliberate, eyes-open departure from ADR-0005 §1's fully-stateless
validation: owner-request validation now depends on **the user's current
`credential_version`**, a per-request read rather than a pure signature check. We
own that explicitly rather than pretend it is free. Three things bound it:

- **The read is not additional on the owner surface.** `requireOwner` already loads
  the user row every request (the ban check, ADR-0009), so the version comparison
  folds into that existing lookup — it adds a column, not a query. The `/admin`
  surface, which also loads the user per request, gets the same check for the same
  reason.
- **No cache ships in this design.** The per-request indexed lookup is the shipped
  mechanism; an in-process `user_id → credential_version` cache is a *documented
  future optimization*, to be added only if profiling shows the version lookup hot.
  Deferring it keeps revocation exact (no staleness window) and avoids the
  multi-node cache-coherence question until there is evidence the read is worth
  optimizing away.
- **The access-token TTL stays short regardless** (ADR-0005 §5). The version check
  is the *immediate* revocation; the short TTL remains the backstop.

The admin capability is unaffected: it is a per-user flag on the same `users` row
(ADR-0010), not a token role, so an admin's sessions invalidate on their own
password change exactly like any owner's.

### 2. Authenticated change-password (settings)

A new authenticated endpoint — `PUT /api/v1/me/password` — takes the **current**
and **new** password. It verifies the current password against the stored
argon2id hash (reusing `internal/auth.VerifyPassword`), applies the shared password
policy (Decision 4), hashes the new password, stores it, and bumps
`credential_version`.

Because the version bump would otherwise log the user out of the very session they
are using, the endpoint **re-issues the acting session** with the new version and
returns it (the frontend swaps the session cookie). Net effect: the caller stays
logged in **on this session only**; every *other* session for that user dies
immediately. The owner Settings UI gains a change-password form that calls this
endpoint.

### 3. Forgot-password reset (unauthenticated, email)

Two anonymous endpoints, following the emailed-capability pattern:

- **Request** — `POST /api/v1/auth/password-reset/request`, body carries an email
  or username. If it resolves to a user who **has a password credential** *and* an
  email sender is configured, the server mints a single-use, **hashed, short-TTL**
  reset token (`internal/token` — raw token emailed, only its hash stored, mirroring
  the reservation-confirm/match-action token tables) and emails a link. The link
  points at a **thin web page** `/reset?token=…` that does nothing but call the
  confirm endpoint — the thin-client shape ADR-0006 already uses for confirm, so a
  future mobile app reuses the same API with universal links and no flow
  duplication.
  - **Enumeration-safe:** the response is **identical** whether or not the account
    exists, has a password, or is emailable — same status, same body, same timing
    characteristics (consistent with the #62 constant-time login work). The server
    never reveals account existence on this endpoint.
- **Confirm** — `POST /api/v1/auth/password-reset/confirm`, body carries the raw
  token and the new password. The server verifies the token (hash match, unexpired,
  unused), applies the shared password policy, sets the new password, marks the
  token used, and **bumps `credential_version`** — so a reset **doubles as
  breach-response for free**: it changes the secret *and* revokes every existing
  session in one step. The confirm response **issues a fresh session so the user
  lands logged in** on the device that completed the reset — the just-set password
  is proof enough of identity, and a forced re-login immediately after would be pure
  friction. Every *other* session is already dead from the version bump.

Reset tokens are single-use and short-lived; the exact table shape and TTL are an
implementation detail of the funnel, modeled on the existing token tables.

### 4. One funnel — a single version-bumping mutation path

All four password-mutation entry points route through **one internal path** that
(a) applies **one shared password policy** (minimum length and any other rules,
defined once) and (b) **bumps `credential_version`** as part of the same write:

1. **`set-password` CLI** (#141, already shipped) — today it only writes the hash
   via `SetPasswordHash`; under this ADR it must **also bump `credential_version`**,
   so an operator reset invalidates stale sessions (the point of #148). This is the
   one small change to already-merged code the ADR requires.
2. **Authenticated change-password** in settings (Decision 2).
3. **Email forgot-password reset** confirm (Decision 3).
4. **`create-owner` CLI** — a new owner starts at version 1; it goes through the
   same funnel so the shared policy applies uniformly (there are no prior sessions
   to invalidate, so the bump is a no-op on first set).

Routing every mutation through one path is what guarantees the two invariants can
never drift apart: **a password never changes without the version moving, and never
gets set in violation of the policy.**

## Consequences

- **Sessions become revocable** at the cost of ADR-0005 §1's zero-DB-read promise:
  authenticated owner requests now read the acting user's `credential_version`. On
  the owner and `/admin` surfaces that read folds into the per-request user load the
  middleware already performs (the ban check), so it adds a column, not a query. No
  cache ships — the exact per-request check is the mechanism, with an in-process
  cache left as a documented future optimization if profiling ever shows it hot
  (Decision 1). This is the central trade-off, and the ADR chooses exact revocation
  over a staleness window. The public/giver surface is unchanged (it never carried a
  session; ADR-0005 §2).
- **Reset and change both invalidate other sessions**, giving #142 a real recovery
  story and #148 its revocation, and making an operator `set-password` a genuine
  lockout/breach-response tool rather than a secret swap an attacker's live token
  ignores.
- **Enumeration stays closed:** the reset-request endpoint reveals nothing about
  account existence, matching the login surface's constant-time posture (#62).
- **A shared password policy is introduced** where none existed; every existing and
  new entry point is subject to it, so a previously-accepted weak password could be
  rejected on its next change (acceptable — it only tightens on mutation, never
  breaks existing hashes).
- **Schema + API additions:** a new `users.credential_version` column and a
  reset-token table (a forward migration, additive), a new `cver` JWT claim, and
  three new endpoints (`PUT /api/v1/me/password`, the two `password-reset/*`) plus a
  thin `/reset` web page — all additive; `api/openapi.yaml` stays the source of
  truth and regenerates on both sides.
- **Token size** grows by one small integer claim — negligible.
- **Refresh tokens remain out of scope** (ADR-0005 §5); this ADR operates entirely
  within the access-only model and is compatible with a later refresh cut, which
  would carry and check the same version.

## Rollout (proposed implementation cuts)

Sequenced so the foundation lands before the flows that depend on it; each is a
separate 2-of-2 PR after this ADR is approved:

1. **Foundation** — `credential_version` column + migration, the `cver` claim, the
   `requireOwner` version check + short-TTL cache, and the single mutation funnel +
   shared policy; retrofit `create-owner` and `set-password` (#141) onto it.
2. **Change-password** — `PUT /api/v1/me/password` + acting-session re-issue +
   Settings form.
3. **Forgot-password** — the two `password-reset/*` endpoints, the reset-token
   store, the emailed link, and the thin `/reset` page (#142).
