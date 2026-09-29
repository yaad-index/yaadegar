# ADR-0016: Personal access tokens

**Status:** Proposed

**Extends** [ADR-0005](0005-owner-authentication.md) (the owner surface and its bearer authentication) and [ADR-0011](0011-password-lifecycle.md) (credential versioning), and is bounded by [ADR-0010](0010-admin-as-a-user-capability.md) (admin as a per-user capability). Supersedes nothing. Addresses #397.

## Context

The owner surface authenticates `Authorization: Bearer <jwt>`, where the JWT is a session minted at login and valid for the access TTL (12 hours by default). That is the right credential for a person at a browser and the wrong one for a script or an integration:

- **A password account** has to store its password in every unattended client and log in again on a schedule.
- **An OAuth-only account has no programmatic path at all.** Login rejects an account with no password hash, and the provider flow ends in a web session, not a bearer token. For these accounts a long-lived token is not a convenience; it is the only way in.
- **Revocation is all-or-nothing.** The only per-user revocation is the `credential_version` bump (ADR-0011), which ends every session and every other client at once.
- **Nothing is attributable.** Sessions are indistinguishable, so there is no way to see which client did something, or whether a client is still in use.

## Decision

### 1. What a token is

A personal access token (PAT) is an **opaque random secret**, minted with `internal/token` (the capability-token pattern): the raw value is shown **once**, at creation, and only its SHA-256 hash is stored. High entropy makes a fast hash sufficient; there is no password-style stretching to do.

The raw value carries a fixed, recognisable prefix (for example `ydg_pat_`). The prefix routes a bearer credential to the right check (§3) without trying to parse it as a JWT first, and it makes a leaked token findable by secret scanners.

Each token is stored with: its owner (tenant and user), a **name** chosen by the user, the **creation time**, an optional **expiry** (no expiry is allowed and must be chosen explicitly), a **last-used time**, a **revoked time**, and the last four characters of the raw value for display. Storage is tenant-scoped through `Store.ForTenant`, like every other tenant record (ADR-0003).

### 2. Where a token is accepted, and with what authority

- **Owner surface (`/api/v1`) only.** A PAT is never accepted on the public surface (ADR-0005 §2, unchanged) and **never on `/admin`**. ADR-0010 lets an owner session authorize `/admin` for an account with the admin capability. Letting a long-lived, copyable credential do the same would turn a script's token into an instance-wide admin credential, so `/admin` accepts sessions only.
- **The account's full owner-surface authority, minus the account's credentials.** Until scopes exist, a PAT can do what the account can do on `/api/v1`, except operate on the credentials of the account itself. It can never:
  - create, list or revoke tokens;
  - change the password (`/me/password`);
  - change the account's email or its linked sign-in identities, should endpoints for either be added.

  A stolen token therefore cannot mint its successor or lock the owner out. This is a rule about the kind of operation, so a new credential endpoint inherits it. The creation screen says plainly that the token carries the account's full access to lists and items.
- **Tenant-scoped.** The token resolves to its owner's home tenant, and the tenant-match invariant (ADR-0005 §5) applies to it exactly as to a session.

Scopes are out of scope for this ADR. When they are added, "full authority minus the exceptions" becomes the default, broadest scope.

### 3. How a token is checked

The bearer middleware keeps a single entry point. A credential with the PAT prefix is checked by hashing it and looking up an unrevoked, unexpired row. Anything else is checked as a session JWT, unchanged. After either path the middleware performs the same per-request user load it already does and applies the same checks:

- **banned** accounts are rejected, so a ban ends every token immediately (ADR-0009 §4);
- the principal is built the same way, so handlers cannot tell the two paths apart except through an explicit "authenticated by token" flag they may use for audit.

The last-used time is written at most once per 5 minutes per token, so a busy script does not turn every request into a write.

### 4. Credential version: tokens do not ride it, with one exception

A PAT is **not** invalidated by the `credential_version` bump of an ordinary password change. For an OAuth-only account the local credential may never change at all, so tying token lifetime to it would couple the token to an event that does not occur. And "change my password" and "revoke my integrations" are separate intents.

**The exception is account recovery.** A forgot-password reset (ADR-0011 §3) is the path taken when the owner may have lost control of the account, and a token minted during that loss of control must not survive it. Completing a reset therefore **revokes every token the account holds**, and the confirmation says so. An ordinary authenticated password change leaves tokens alone, and shows the number of active tokens with a link to review them.

### 5. Creating a token requires recent authentication, by any method

Creating a token requires that the acting session was issued within the last **10 minutes**. Sessions are not refreshed (ADR-0005 §5), so a session's issue time is the time of its last authentication. A user whose session is older logs in again with whichever method they use: password, OAuth or magic link.

This deliberately avoids a password-confirmation step, the conventional safeguard, because an OAuth-only account has no password to confirm, and it is exactly those accounts that need tokens most. If sessions ever become refreshable, the refresh mechanism must carry the original authentication time forward, or this check weakens silently.

### 6. Limits and rate limiting

- An account may hold at most **20** active tokens. Creation beyond that is refused with a clear error.
- **Failed** PAT authentications (well-formed prefix, no matching active token) count against a per-client-IP limiter of their own, separate from the login limiter. Guessing a high-entropy token is not feasible; the limiter bounds the lookup load and makes probing visible. Successful token requests are not rate-limited by this ADR.

### 7. Visibility

- The owner's settings list every token with its name, last four characters, creation time, expiry and last-used time, and revoke individually. Revocation takes effect on the next request.
- Creation and revocation are logged with the token's id and name, never its value.

## Consequences

- One new table, one new branch in the bearer middleware (sharing the user load and every check after it), a settings screen, and an explicit exclusion of tokens from `/admin`.
- OAuth-only accounts gain programmatic access for the first time.
- A password change no longer revokes everything an account can authenticate with. The deliberate exception is recovery, which revokes all tokens.
- The recent-authentication rule (§5) puts a new constraint on any future session-refresh design.
- Tests the implementation must include:
  - a token authenticates on `/api/v1` and is rejected on `/admin` and on the public surface;
  - a revoked or expired token is rejected;
  - a banned account's token is rejected;
  - an ordinary password change leaves tokens valid, and a forgot-password reset revokes them all;
  - token creation is refused on a session older than 10 minutes and accepted after a fresh OAuth login;
  - a token cannot create or revoke tokens or change the password;
  - the stored record never contains the raw value.
