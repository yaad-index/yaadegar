# ADR-0009: Unified identity, roles, and registration

**Status:** Proposed

## Context

Three earlier ADRs each fixed one slice of "who is a principal":

- [ADR-0005](0005-owner-authentication.md) established the owner session (JWT), the
  instance **superadmin** (a distinct instance-level identity on a separate
  `/admin` surface, authorized by role, carrying a sentinel tenant id), the
  `list_owners` join model, and the guest tiers — with the **`registered`**
  reserver tier explicitly deferred because there were no giver accounts to back it.
- [ADR-0007](0007-reserver-identity-policy.md) shipped the reserver tiers
  (`full_guest` / `email_confirmed` / `registered`), per-list override, and the
  email-confirmation flow — again leaving `registered` inert.
- [ADR-0008](0008-owner-oauth.md) made owner login **link-only**: a Google identity
  attaches to an *existing* owner; self-registration / auto-provisioning was deferred.

So today: owners are provisioned by the operator (CLI, or the admin API) and log in;
givers are anonymous (capability tokens, ADR-0002 §5); the instance admin is a
separate configured identity; and `registered` is a tier with nothing behind it.

This ADR unifies those into one identity model and adds **self-registration** and an
**admin user-management** surface. The operator's target shape:

- **admin** — a per-instance capability (a flag), not a tenant-scoped role.
- **owner** — a list owner, who may **also** act as a giver.
- **giver** — promoted from an anonymous reserver to a **first-class account**, which
  is what finally makes the deferred `registered` reserve tier real.
- an instance **self-registration policy**: `disabled` / `givers_only` /
  `owners_allowed`.
- an **admin page**: change a user's role (giver ↔ owner), ban a user, and create a
  user by email.

Because an ADR is immutable, the points below that touch ADR-0005 §6 and ADR-0008 §5
are stated here as **refinements**, not edits to those files.

## Decision

### 1. Two orthogonal axes, not one ladder

Identity is modeled on **two independent axes**, which keeps the operator's "three
groups" from collapsing into a single confused hierarchy:

- **Per-tenant role** on a user account: **`owner`** or **`giver`**. This lives on
  the tenant-scoped `users` row (a new `role` column) and governs what the account may
  do *within its tenant*: an owner owns lists (via `list_owners`) and reaches the full
  owner surface; a giver may authenticate and reserve/contribute but owns no lists.
- **Instance-admin capability** — an orthogonal, instance-level flag (Decision 3),
  never a tenant role. It grants the non-tenant-scoped `/admin` surface and nothing on
  `/api/v1` by itself.

An **owner can also give** for free: an owner is an authenticated account, so it can
use the authenticated reserve path (Decision 5) — no separate "giver" account needed.
The account's tenant role is the *floor* of its abilities, not a mutually-exclusive
bucket.

### 2. Self-registration policy (instance-level)

A new instance config selects the registration policy, defaulting to the **current,
safest** behaviour:

- **`disabled`** (default) — no public signup; accounts are provisioned by an admin or
  an existing owner (today's model + ADR-0008 link-only). A fresh instance behaves
  exactly as it does now.
- **`givers_only`** — anyone may self-register, but a self-registered account is a
  **giver** (no list creation). This is what turns on `registered`-tier reserving for
  the public without opening list-authoring.
- **`owners_allowed`** — a self-registered account may create lists (becomes a
  **owner**). Full open registration.

The policy is a per-instance setting (env / config, like the other instance knobs), not
per-tenant, and is enforced server-side at the signup endpoint. Signup reuses the
ADR-0005 method surface (password now; magic-link / OAuth self-registration compose
later) and the same session issuer.

### 3. The admin flag and ADR-0005's superadmin are ONE concept

ADR-0005 modeled the instance admin as `role=superadmin` on a sentinel tenant. The
operator wants "a per-instance flag." These are the **same principal**; this ADR
**does not add a second admin concept**. The reconciliation:

- Keep exactly one instance-level administrator concept: the **instance admin**,
  authorized on the existing non-tenant-scoped `/admin` surface, bootstrapped by config
  as today (ADR-0005 §6 — a configured identity with a hashed credential, no open
  bootstrap endpoint).
- Re-describe it internally as a **capability/flag** rather than a parallel role ladder,
  so there is no `admin` value competing with `owner`/`giver` on the tenant role axis.
  The instance admin is orthogonal to every tenant role (Decision 1).

**Recommendation:** treat instance-admin as the single existing superadmin capability;
do **not** grant instance-admin implicitly to any tenant role, and do not create a
tenant-level `admin` role. Whether to *rename* the internal `superadmin`/`RoleSuperadmin`
symbols to `instance-admin` is a churn-vs-clarity call — **recommend keeping the
internal names** (as with `owner`, Decision 2) and only presenting "instance admin" in
docs/UI, to avoid a mechanical rename touching auth-critical code. Optionally, a later
cut may let an admin *grant* the instance-admin capability to a normal user account;
v1 keeps the configured bootstrap admin as the sole holder.

### 4. Admin user-management surface

The `/admin` surface (instance-admin only) gains user management:

- **Create a user by email** — provision an `owner` or `giver` account for an email,
  without a password (the user sets one via the enabled login method — password reset /
  magic-link / OAuth link, composing with ADR-0005 / ADR-0008). Supersedes, but does not
  remove, the `create-owner` CLI (which stays for headless ops).
- **Change role** — flip a user's tenant role `giver ↔ owner`. Because owner access
  flows entirely through `list_owners`, demotion to `giver` **must also settle the
  account's ownership rows** — a demoted giver whose `list_owners` rows linger would keep
  owner-level access. This is therefore a **hard precondition for the Cut 1 change-role
  spec** (see Rollout), not a downstream open item: the spec must resolve it (e.g. reject
  demotion while the account owns lists, or revoke/reassign the rows in the same
  operation).
- **Ban** — a `banned` flag on the user: a banned account cannot log in or hold a
  session. It is enforced at two points — at token **issue** (a banned account gets no
  new session) and at the owner middleware's **existing per-request user load**
  (`requireOwner` already reads the user row, so the flag takes effect immediately on
  the owner surface, with no revocation store needed). The only residual window is a
  purely-stateless token check that does *not* reload the user: there the access token
  stays valid until its short JWT TTL (ADR-0005 §5) expires. Ban is reversible; it is not
  deletion.

### 5. A registered giver satisfies the `registered` reserve tier

This is the payoff that was deferred in ADR-0005 §6 and ADR-0007. When a list's
**effective** reserver tier is `registered`:

- The reservation must be created by an **authenticated account** (a JWT principal —
  `owner` or `giver` — resolved for the tenant), instead of an anonymous capability
  token. The account is the proof of the `registered` tier; ADR-0005 §6 anticipated
  exactly this ("a future `registered` reserver that carries an account would issue a
  JWT through the same issuer").
- **Anonymity is preserved (ADR-0002 §5).** Binding the reservation to a `user_id`
  server-side gates the tier; it does **not** disclose the reserver's identity to the
  owner. The owner still never learns who reserved. (Co-buying's post-match reveal is
  unchanged — that is an explicit, mutual opt-in, ADR-0002 §6.)
- The lower tiers are unaffected: `full_guest` and `email_confirmed` keep the anonymous
  capability-token path (Decision 1). A registered account may also reserve on lists
  set to a lower tier — the tier is a floor.

### Operator decisions

Decision 1 is **settled** (operator-confirmed); decisions 2–5 carry a recommendation
for the operator to confirm.

1. **Registered giving is a THIRD tier ALONGSIDE the existing two — DECIDED.**
   `registered` joins `full_guest` (+ the #45 anti-bot captcha) and `email_confirmed`
   as an additional reserve tier; the identity work **only adds** `registered` and
   leaves `full_guest` and `email_confirmed` untouched. Anonymous / email-only giving —
   the core low-friction gifting UX and the anonymity invariant (ADR-0002 §5) — is
   preserved; registered accounts add a tier and a saved identity for people who want
   one. (This also settles Decision 5's shape: the authenticated path serves the new
   tier without disturbing the anonymous ones.)

2. **Naming — SETTLED (operator-confirmed): keep `owner` as both the internal and the
   display term.** The tenant role axis is **`owner` | `giver`**; there is no separate
   display word (Owner / Curator are dropped) and no new `owner` role value. This
   reuses the existing `RoleOwner`, `list_owners`, `ownsList`, and the `/api/v1` shape
   unchanged — even less churn than a rename, and one word for the concept everywhere.

3. **Admin vs superadmin.** **Recommend one concept** — the instance admin is exactly
   ADR-0005's superadmin, re-described as a capability, kept orthogonal to tenant roles,
   with internal symbol names unchanged (Decision 3 above).

4. **Migration from superadmin-creates-owner.** **Recommend additive, zero-loss:** add a
   `role` column to `users` defaulting **`owner`** (every existing user is an owner
   today, so they all become owners); add a `banned` flag defaulting false; leave
   `list_owners`, the `admins`/superadmin bootstrap, and the CLI provisioning untouched.
   Self-registration and giver accounts are purely additive; no existing account changes
   behaviour.

5. **How a registered giver satisfies `registered`.** **Recommend the authenticated
   reserve path** (Decision 5): an authenticated account's reservation counts as
   `registered`, bound to `user_id` server-side, anonymity preserved.

### Config and defaults

- **Instance default reserver tier → `email_confirmed` (operator-confirmed).** A list
  with no per-list override currently inherits the instance default, which today is
  **`full_guest`** (`--reserver-default-tier` / `YAADEGAR_RESERVER_DEFAULT_TIER`,
  default `full_guest`; ADR-0007). The operator has decided the default should be
  **`email_confirmed`**, so a fresh list starts requiring a verified email rather than
  fully anonymous reserving. This is a one-line default change (config + the documented
  example) and is small enough to ship as its own cut, independent of the identity
  work; per-list overrides (the #126 control) and the two lower tiers are unchanged.
- The new instance settings this ADR introduces — the **self-registration policy**
  (Decision 2) — follow the same instance-config convention (env / config file, not
  per-tenant), defaulting to `disabled` so behaviour is unchanged until an operator
  opts in.

## Consequences

- New: a `role` column and a `banned` flag on `users` (migration); an instance
  self-registration-policy config; a public signup endpoint (gated by policy); admin
  user-management endpoints (create-by-email, change-role, ban) on `/admin`; an
  authenticated reserve path for the `registered` tier; and the admin + signup + giver
  frontend surfaces. Spec-first for every new endpoint (ADR-0002).
- Refines ADR-0005 §6 (the role model gains an explicit `owner`/`giver` tenant axis
  and re-frames superadmin as the instance-admin capability) and ADR-0008 §5 (link-only
  is no longer the only account-creation path once self-registration is enabled;
  link-only remains the behaviour when the policy is `disabled`).
- Unchanged and load-bearing: the two-surface boundary and JWT/capability separation
  (ADR-0005 §2), the tenant-match invariant (ADR-0005 §5), reserver anonymity (ADR-0002
  §5), and the co-buy reveal (ADR-0002 §6).
- Deferred / open items to settle at cut-scope time (called out, not baked):
  - abuse controls on open signup (captcha #45, rate-limit, email verification before a
    self-registered account is usable) — lean on the existing anti-bot boundary
    (ADR-0005 §8);
  - cross-tenant identity (one person with accounts in several tenants) stays out of
    scope — accounts remain tenant-scoped, as today.

## Rollout (proposed cuts, after sign-off — each its own PR, 2-of-2, spec-first)

1. **Cut 1 — admin user management.** The `role` + `banned` columns (migration,
   defaults preserving today's behaviour), the `/admin` create-by-email / change-role /
   ban endpoints, the instance-admin capability reconciliation, and the admin
   user-management UI. No self-registration yet. Unblocks the operator managing users.
   **Precondition:** the change-role spec must resolve demotion vs `list_owners` access
   (above) before the endpoint is written — since owner access flows through
   `list_owners`, demotion must revoke or reassign ownership (or be rejected while lists
   exist), so a demoted account cannot retain owner access.
2. **Cut 2 — self-registration.** The instance policy config (`disabled` /
   `givers_only` / `owners_allowed`), the policy-gated public signup endpoint
   provisioning a `giver` or `owner`, and the signup UI. Composes with the ADR-0005
   methods and ADR-0008 link.
3. **Cut 3 — registered-giver reserve.** The authenticated reserve path that satisfies
   the `registered` tier (bound to `user_id`, anonymity preserved), and the
   giver-account reserve UX. Makes ADR-0007's `registered` tier real end to end.

No code lands until this ADR is signed off. Cuts are sized here but each is reviewed and
gated on its own.
