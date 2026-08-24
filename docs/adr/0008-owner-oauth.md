# ADR-0008: Owner login via Google OAuth / OIDC

**Status:** Accepted

## Context

[ADR-0005 §3](0005-owner-authentication.md) established that three owner login
methods — username+password, magic-link, and **Google OAuth** — all converge on
one `internal/auth` session issuer, and named Google OAuth as "the largest method
[that] lands last." This ADR is that method (#21): it fixes the OIDC flow, the
multi-tenant redirect handling, the account model, and the config, so the
implementation is not discovering architecture as it goes.

Two points in ADR-0005 §3 are **refined** here for v1 (an ADR is immutable, so the
refinements are stated below rather than edited into §3):

1. §3 said "**per-tenant client credentials**." Yaadegar is self-hosted-first —
   one operator per instance — so per-tenant client apps (each tenant registering
   its own Google project) are a true-SaaS feature that does not fit v1. **v1 uses
   a single instance-level Google client** (operator-configured) with a **per-tenant
   on/off toggle**. Per-tenant credentials are deferred to a later ADR.
2. §3 said the ID token's email is "**create-or-link** within the tenant." Auto-
   provisioning an owner (or tenant) from a Google login is a signup path with its
   own abuse surface. Owners are operator-provisioned (ADR-0005 A2b-2). **v1 is
   link-only**: a Google login attaches to an *existing* owner or is rejected.
   Self-registration is deferred.

OAuth is purely **add-on** — it never changes password auth, the session token, or
the middleware. An owner may have a password, a Google identity, or both.

## Decision

### 1. Flow — OIDC Authorization Code with PKCE

Standard OpenID Connect Authorization-Code flow with PKCE (S256) and a `nonce`.
The library does the protocol: **`golang.org/x/oauth2`** for the code exchange and
**`github.com/coreos/go-oidc/v3`** for provider discovery + **ID-token
verification** (JWKS signature, `iss`, `aud`, `exp`, `nonce`). We do not hand-roll
JWKS fetching or JWT verification.

### 2. Multi-tenant redirect — one fixed redirect_uri, tenant carried in state

Tenants live on many hosts (per-tenant subdomains and custom domains, ADR-0004), so
we cannot pre-register every host as a Google `redirect_uri`. Instead there is a
**single fixed redirect_uri** on the instance's canonical host
(`YAADEGAR_OAUTH_REDIRECT_BASE`), and both the start and callback endpoints live on
that host:

- `GET /api/v1/auth/oauth/google/start?tenant=<subdomain-or-host>&return_to=<path>` —
  validates the tenant hint, mints `state` + PKCE verifier + `nonce`, stores them in
  a signed cookie **on the fixed host**, and redirects to Google.
- `GET /api/v1/auth/oauth/google/callback?code&state` — lands on the fixed host
  where the cookie is readable; validates `state` (CSRF), exchanges the code with
  the PKCE verifier, and verifies the ID token (§4).

The tenant is carried inside the signed `state`, so the callback knows which tenant
the login is for without trusting any request-supplied host.

### 3. Cross-host session handoff — a one-time signed ticket

The session cookie must be **host-scoped to the owner's tenant host** (a
`Domain=.instance` parent-domain cookie would be sent to *every* tenant subdomain —
a cross-tenant session leak — and does not span custom domains at all). But the
callback runs on the fixed host. So the handoff is:

1. On success the callback mints a **short-TTL, single-use, HMAC-signed ticket**
   (same key material as the state cookie) encoding `{tenant, user_id, exp, jti}`.
2. It `302`s to `https://<tenant-host>/api/v1/auth/oauth/complete?ticket=…` (tenant
   host taken from the signed state, never from a request parameter).
3. `GET …/oauth/complete` on the **tenant host** validates the ticket (signature,
   expiry, single-use), mints the normal session JWT via the ADR-0005 issuer, sets
   the **host-scoped** httpOnly session cookie, and redirects to the dashboard.

This works uniformly for subdomain and custom-domain (ADR-0004 / #12) tenants with
no rework, and never lets a session cookie escape its tenant host.

### 4. ID-token verification (the security core)

Before any account lookup, the ID token must pass **all** of: JWKS signature valid;
`iss` == `https://accounts.google.com`; `aud` == our client id; `exp` in the future;
`nonce` == the nonce from the state cookie. go-oidc performs these; a failure aborts
the login. Only then are `sub` (stable Google subject), `email`, and
`email_verified` read.

### 5. Account model — link-only, verified-email, with four guards

A verified Google identity is linked to an **existing** owner in the tenant, or the
login is rejected. There is no auto-provisioning in v1. Four guards close the
account-takeover surface:

1. **`email_verified == true`** required — an unverified Google email cannot link
   (this is what closes the email-collision path: a Google-verified email is proof
   Google controls that mailbox).
2. **Tenant-scoped linking** — the identity attaches only within the tenant resolved
   from the signed state; a Google login on tenant A can never touch tenant B.
3. **Full ID-token verification** (§4).
4. **`unique(tenant_id, provider, subject)`** — within a tenant, one Google account
   maps to at most one owner. The constraint is **tenant-scoped, not global**: the
   same Google account may legitimately own in two different tenants on one instance
   (e.g. an operator running two tenant configs); global uniqueness would wrongly
   block that, and the takeover protection comes from guards 1–2, not from global
   subject uniqueness.

Linking rule: on callback, look up an owner in the tenant whose stored email equals
the verified Google email. If found and not already linked to a *different* Google
subject → record the identity and issue the session. If none → reject with a clear
"no owner with this email on this list" message (the operator must provision the
owner first).

### 6. Data model

New table (migration 0014), owner FK named **`user_id`** to match the existing
schema (`list_owners.user_id` → `users.id`):

```
oauth_identities(
  id, tenant_id, user_id, provider, subject, email, created_at,
  UNIQUE(tenant_id, provider, subject)
)
```

Per-tenant enablement is a boolean toggle (`oauth_google_enabled`, default off) on
the tenant's settings, resolved through the existing `settings.Resolve` pattern. The
toggle is inert unless the instance has a configured Google client (§7).

### 7. Config and fail-closed startup

Instance config via environment (the SMTP pattern; secrets env-only, never inlined):

- `YAADEGAR_OAUTH_GOOGLE_CLIENT_ID`
- `YAADEGAR_OAUTH_GOOGLE_CLIENT_SECRET`
- `YAADEGAR_OAUTH_REDIRECT_BASE` (the fixed https host serving `/start` + `/callback`)

This extends the ADR-0005 §4 fail-closed startup: if Google OAuth is enabled but any
of the three is missing/blank, the instance **refuses to start**, naming the field.
A per-tenant toggle set on an instance with no configured client is a no-op (the
method is simply unavailable), not a startup failure.

### 8. Session/state cookie

The `/start` → `/callback` cookie holding `{state, pkce_verifier, nonce, tenant,
return_to}` is HMAC-signed, **httpOnly**, **Secure**, **SameSite=Lax** (Lax so it
survives the top-level redirect back from Google), short-TTL, and set on the fixed
host only. It is not a session and grants nothing on its own.

## Operator prerequisites (build gates)

Implementation does **not** start until both land:

1. **Sign-off on this ADR** (the hard gate for ADRs).
2. **A Google OAuth client** — client id + secret, plus the **single registered
   `redirect_uri`** pointing at `YAADEGAR_OAUTH_REDIRECT_BASE`'s callback path.

**HTTPS note (Google requirement).** Google only accepts **https** redirect_uris
(the sole exception is `http://localhost`). The current demo
(`http://…nip.io:8095`, plain http) therefore **cannot** be a registered redirect_uri
as-is. Two options — the ADR asks the operator to pick one:

- **(A) Give the demo an https endpoint first** (e.g. Tailscale serve fronting it
  with a cert), and register that as the redirect_uri. OAuth is then exercised on
  the live demo. **Recommended if the https front is cheap.**
- **(B) Validate OAuth via `http://localhost` + integration tests only**, ship it
  behind the per-tenant toggle (off by default), and enable it on the demo later
  once an https endpoint exists.

Either way the feature can merge and be test-covered independently of the demo's
https timeline; (A) only affects whether it is *exercised live* now.

## Consequences

- Two new unauthenticated endpoints on the fixed host (`/oauth/google/start`,
  `/oauth/google/callback`) and one on the tenant host (`/oauth/google/complete`),
  all outside the JSON strict server (redirects, not JSON) — the same raw-handler-on-
  the-mux pattern used for import/export (#26), behind the same middleware.
- Two new dependencies: `golang.org/x/oauth2`, `github.com/coreos/go-oidc/v3`.
- Password auth (ADR-0005), the session token, and the middleware are untouched.
- Custom-domain tenants (ADR-0004) are covered by the ticket handoff with no rework.
- Deferred: per-tenant client credentials; OAuth-driven self-registration /
  auto-provisioning; providers other than Google (the schema's `provider` column
  and the `/oauth/{provider}/…` path leave room, but only Google is wired in v1).

## Rollout (proposed implementation cuts, after sign-off + client)

- **Cut 1 — backend:** `oauth_identities` + toggle + migration 0014; the three
  endpoints; go-oidc verification; the ticket handoff; env config + fail-closed
  startup extension; link-only account model with the four guards. Integration-tested
  against a mock OIDC provider (no live Google in CI).
- **Cut 2 — frontend:** a "Sign in with Google" button on the owner login page
  (visible only when the tenant toggle is on and a client is configured), the
  redirect wiring, and the owner-settings toggle UI.
