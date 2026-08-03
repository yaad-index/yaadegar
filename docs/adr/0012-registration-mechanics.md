# ADR-0012: Registration mechanics

**Status:** Accepted (2026-08-03)

**Refines** [ADR-0009](0009-identity-roles-and-registration.md) (unified identity,
roles, and registration): it fills in ADR-0009's deferred **Cut 2**
(self-registration) and **Cut 3** (registered-giver reserve) with the concrete
signup, email-verification, and reserve mechanics. Builds on
[ADR-0008](0008-owner-oauth.md) (link-only Google login),
[ADR-0007](0007-reserver-identity-policy.md) (reserver tiers + the #45 CAPTCHA
low-trust boundary), and [ADR-0011](0011-password-lifecycle.md) (the credential
model, the "no password set" state, and the hashed single-use token store).
Supersedes nothing.

**This ADR does not change ADR-0009's policy or role model.** The instance
registration policy stays `disabled` / `givers_only` / `owners_allowed`; the role
axis stays `owner | giver` with owner ⊇ giver (an owner may give, a giver owns no
lists); a new registrant's role is set by the policy; and promotion `giver → owner`
stays the admin/CLI change-role from ADR-0009 Cut 1. Everything here is additive
mechanics on top of that settled model.

## Context

ADR-0009 unified the identity model and sketched three rollout cuts. Cut 1 (admin
user-management: the `role` + `banned` columns, `/admin` create-by-email /
change-role / ban) has shipped. Cuts 2 and 3 were sized but left as "no code until a
follow-up," and ADR-0009 explicitly deferred the abuse-control and account-usability
details to "cut-scope time":

- **Self-registration (Cut 2)** — the policy config exists in the design, but *how* a
  person actually signs up (which methods, what proves they control the email, what
  stops bots) was left open. ADR-0009 §Consequences names "captcha #45, rate-limit,
  email verification before a self-registered account is usable" as items to settle
  here.
- **Registered-giver reserve (Cut 3)** — ADR-0009 Decision 5 established *that* an
  authenticated account's reservation satisfies the `registered` tier with anonymity
  preserved, but not the end-to-end UX (which reserve path, whether a confirm email
  fires, where the reserver sees their own reservations).

Since ADR-0011 shipped, two mechanics that Cut 2/3 need already exist and should be
reused rather than reinvented: the **"no password set" state** (an empty password
hash, which the change-password handler already rejects with a 403) and the
**hashed, single-use, short-TTL token store** (`password_reset_tokens`) with its
atomic single-use claim. This ADR records the registration mechanics as one coherent
design so the three cuts can be built against it. It is design-only; no code lands
until it is approved.

## Decision

### 1. Two self-registration methods, both gated by the ADR-0009 policy

Self-registration offers two methods. **Both are gated by the ADR-0009 instance
policy**: `disabled` blocks both; `givers_only` creates a `giver`; `owners_allowed`
creates an `owner`. A new account's role is the policy role — this ADR adds no new
policy value and no new role.

**1a. Google OAuth self-register (one click, no password, no email verification).**
A person who is not yet an account clicks "Sign in with Google," and — when the
policy allows it — an account is **auto-created at the policy role**. No password is
prompted (see Decision 2) and no email-verification step runs: Google has already
proven the person controls that mailbox (`email_verified == true`, ADR-0008 §5 guard
1), which *is* the verification. OAuth is the trust gate, so this path also **skips
the CAPTCHA** (consistent with ADR-0007 §4: an authenticated/OAuth path is not
low-trust).

This **reconciles with ADR-0008's link-only rule** by extending the callback's
match-or-reject into match-or-create-or-reject. On the OAuth callback, after the
existing ID-token and tenant-scoping guards (ADR-0008 §4–5):

- if the verified Google email matches an **existing** account in the tenant → **link
  and log in** (unchanged ADR-0008 §5 behaviour, including the "already linked to a
  different subject" rejection);
- else if the instance **policy allows self-register** → **create a new account** at
  the policy role, record the OAuth identity, and log in;
- else (policy `disabled`, no match) → **reject** with the existing ADR-0008
  "no owner with this email" message.

This is a refinement of ADR-0008 §5: link-only remains the behaviour whenever the
policy is `disabled`; auto-provisioning is the *only* new branch, and only when the
policy opts in.

**1b. Email + password self-register (CAPTCHA + email-link verification).** A person
submits an email, a password (subject to the ADR-0011 shared password policy), and a
**CAPTCHA** token. This is the low-trust path, so it carries the #45 CAPTCHA (ADR-0007
§4) verified server-side, **and** an email-ownership check: the account is created in
a **pending / inactive** state and **cannot log in** until the person clicks a single-
use verification link emailed to that address, which activates the account. Only then
is a session issuable. This closes the "sign up as someone else's email" surface — an
account is usable only once its email is proven.

> **Lean (for sign-off):** the CAPTCHA is required on the **email+password path only**;
> the OAuth path skips it (OAuth is the trust gate). This matches ADR-0007 §4's
> low-trust definition.

### 2. No-password accounts use the real "no password set" state (empty hash)

An OAuth-self-registered account — and any account created without a password — is
stored with the genuine **"no password set" state: an empty `password_hash`**, *not*
a random unusable blob. This is exactly the state ADR-0011 already defines and
handles:

- The ADR-0011 Cut-2 change-password handler already returns **403 "this account has
  no password set"** for an empty hash, so change-password is naturally unavailable to
  these accounts with no new code.
- To *establish* a password later, a no-password account uses the **same email-token →
  set-password → auto-login machinery** ADR-0011 Cut 3 built: the confirm sets a
  password and bumps `credential_version` through the shared funnel. "Set a first
  password" and "reset a password" are the same mechanic on the storage/token side — no
  separate password-setting engine is needed.

Using the empty-hash state (rather than a random blob) keeps one meaning for "no
password," avoids a magic sentinel, and makes the change-password handler behave
correctly for these accounts by construction (it already 403s on an empty hash).

**One reconciliation the cut must handle (surfaced in review).** The ADR-0011
*request* endpoint was deliberately scoped to *reset an existing* password: its
`resettable()` guard requires a **non-empty** `password_hash` (plus a deliverable
email, not banned). So a no-password account cannot today receive a link through
`POST /api/v1/auth/password-reset/request` as written — it would hit the
enumeration-safe silent 202 and send nothing. Establishing a *first* password
therefore needs one of two cut-scope choices, both reusing the ADR-0011 hashed
single-use token store and set-password/auto-login confirm (not a new engine):

- **widen the entry guard** so a no-password-but-emailable account is eligible for an
  establish/reset link (the request path serves both "set first" and "reset"); or
- **add a distinct "establish password" / invite entry** (e.g. the admin-invite send of
  Decision 6, and an account-initiated equivalent) that mints the same token without
  the has-password precondition, leaving `RequestPasswordReset`'s reset-only guard
  untouched.

Either keeps the confirm side identical; the split is only about *who is allowed to be
sent a link*.

> **Lean (for sign-off):** reuse the empty-hash state + the ADR-0011 token/confirm
> machinery for first-password establishment (rather than a separate password-setting
> engine); the entry-point choice above — widen the guard vs. a distinct establish/invite
> entry — is a cut-scope decision.

### 3. Email-verification token store + pending-account state

Email verification (Decision 1b) needs a token store and an account state:

- **Token store** — mirror the ADR-0011 Cut-3 `password_reset_tokens` design: a new
  table for email-verification tokens holding a **sha256 hash** of the raw token (the
  raw is emailed once, never stored, ADR-0003 §3), **single-use** via the same atomic
  `used_at IS NULL` claim, and a **short TTL** (expiry checked in Go). A new migration
  adds it. The two token stores stay separate tables (distinct purposes and
  lifetimes) but share the shape.
- **Pending-account state** — a self-registered email+password account is **pending**
  (not yet activated) until its email is verified. A pending account cannot log in and
  cannot reserve. Verifying the token flips it to active. The exact representation (a
  dedicated `status`/`activated_at` column vs. reusing an existing flag) is an
  implementation detail of the cut.

> **Lean (for sign-off):** the pending-account representation and the verification-
> token **TTL** (proposed on the order of the reset-token TTL — short, hours not days)
> are set at cut-scope time; flagged here so the value is a conscious choice, not a
> default.

### 4. Registered-tier authenticated reserve + a reserver dashboard

This makes ADR-0007's `registered` tier and ADR-0009 Decision 5 real end to end. When
a list's **effective** reserver tier is `registered`:

- The reservation is created through an **authenticated reserve path**, bound to the
  caller's `user_id` **server-side** from their session (not an anonymous capability
  token). The account is the proof of the `registered` tier (ADR-0009 Decision 5).
- **No per-reservation confirmation email** fires: the account's email was already
  verified at registration (Decision 1), so the `email_confirmed`-style per-
  reservation confirm step (ADR-0007) is redundant for a registered reserver.
- The reservation shows up in the **reserver's own dashboard** — the deferred ADR-0007
  "things I've reserved" view — so a registered giver can see and manage what they
  reserved across lists, keyed on their account.
- **Anonymity to the owner is preserved (ADR-0002 §5).** Binding the reservation to a
  `user_id` gates the tier and powers the reserver's *own* view; it discloses nothing
  to the **owner**, who still never learns who reserved. Co-buying's post-match reveal
  is unchanged (an explicit mutual opt-in, ADR-0002 §6).
- The lower tiers are untouched: `full_guest` and `email_confirmed` keep the anonymous
  capability-token path. A registered account may still reserve on a lower-tier list —
  the tier is a floor (ADR-0009 Decision 5).

> **Lean (for sign-off):** the interaction of an authenticated/registered reservation
> with **co-buying and decay** — a registered reserver's contribution, and whether an
> account-bound reservation participates in the decay reminder flow the same way an
> email-bound one does — is called out as a cut-scope question, not baked here. The
> default lean: a registered reservation follows the same decay lifecycle, with the
> dashboard (not only email) as an additional keep/release surface.

### 5. The per-list "registered-only" reserve option is decoupled from the instance policy

An owner may set a list's effective tier to `registered` (the #126 per-list control)
**regardless of the instance registration state.** The per-list option is **not** hard-
blocked when instance registration is `disabled`:

- When instance registration is **disabled** and an owner sets a list to
  `registered`, the option's dialog shows a **warning**: only operator-created accounts
  can reserve on this list, because the public cannot self-register. The owner may
  still proceed (an operator-provisioned-accounts instance is a legitimate closed-
  registration deployment).
- When registration is `givers_only` / `owners_allowed`, the same option needs no
  warning — the public can register and therefore reserve.

Decoupling the control from the instance toggle avoids a confusing hard dependency (an
owner setting a tier and having it silently rejected by an instance knob they don't
control) and keeps the per-list tier a pure owner choice, with the instance state
surfaced as guidance rather than a gate.

### 6. One unified onboarding / first-password flow

Admin-created accounts (the ADR-0009 Cut-1 create-by-email surface) and **any**
no-password account onboard through **one mechanism** — the same one ADR-0011 Cut 3
already built for forgot-password:

- An account starts with **no password** (empty hash, Decision 2) and receives a
  **single-use email link** that **sets its first password and auto-logs it in** —
  exactly the ADR-0011 email-token → set-password (through the shared funnel, which
  bumps `credential_version`) → auto-login machinery, no new pathway.
- "Create user" (admin) **sends that invite / set-password link**. Clicking it both
  sets the password **and proves email ownership**, so for an admin-invited account
  the same click doubles as email verification — no separate verification step.
- Alternatively, the invited person **links Google OAuth** instead of setting a
  password (Decision 1a / 2); the account simply stays no-password.

So there is **no temporary password emailed** and **no "must change password" flag** —
both are explicitly rejected (see Considered alternatives). This unifies three things
that would otherwise each grow their own flow — **forgot-password, admin-invite, and
OAuth-account onboarding** — into the single email-token → set-password → auto-login (or
OAuth-link) mechanism, and it matches ADR-0009's existing "create-by-email without a
password; the user sets one via reset / magic-link / OAuth" direction (ADR-0009 §4).

Because these invited/onboarding accounts start at the empty-hash no-password state,
sending them a set-password link runs into the **Decision 2 reconciliation** (the
ADR-0011 `resettable()` guard requires a non-empty hash): the cut must either widen that
guard or expose a distinct establish/invite entry that reuses the ADR-0011 token store.
This onboarding flow is the concrete reason that reconciliation matters.

> **Lean (for sign-off):** this unified invite/set-password flow is the recommendation,
> **pending the operator's pick** between it and the temp-password route below. The two
> are presented as a Considered alternative for that choice.

## Considered alternatives

**Emailed temporary password + a forced password change (rejected; the alternative the
operator is choosing against for Decision 6).** Instead of an invite link that sets the
first password, the system emails a **temporary password**, the user logs in with it,
and a **`must_change_password` flag** forces them to set a real one on first login.

Rejected because:

- **Emailing a password is insecure** — even a temporary one is a live credential
  sitting in the mailbox (and any mail logs/relays) until used; it is exactly the
  "never email a secret" anti-pattern. The invite link carries a **single-use,
  short-TTL, hashed** token instead (ADR-0011 / ADR-0003 §3), which sets the password
  over a form and stores nothing reusable.
- **The force-change flag is needless persistent state** — an extra account column and
  a login-time branch that the invite-link flow does not need: with the link flow the
  account simply has no password until the user sets one, which is already a
  first-class state (Decision 2).
- It would be a **fourth** onboarding path rather than reusing the ADR-0011 mechanism,
  cutting against Decision 6's unification.

The invite-link flow also proves email ownership as a side effect (the link only
reaches the real mailbox), which the temp-password flow only achieves incidentally.

## Consequences

- **New endpoints (spec-first, ADR-0002 — shapes finalized per cut):**
  - `POST /api/v1/auth/register` — email + password + CAPTCHA self-register (Decision
    1b); creates a pending account and emails a verification link. Policy-gated.
  - `POST /api/v1/auth/register/verify` — consume the emailed verification token,
    activate the account (Decision 1b/3); may auto-issue a session so the person lands
    logged in (mirrors ADR-0011 reset auto-login — a lean to confirm).
  - **OAuth self-register** adds no new endpoint: it is the existing ADR-0008 Google
    callback extended with the create-on-allowed-policy branch (Decision 1a).
  - An **authenticated reserve path** for the `registered` tier (Decision 4), bound to
    the session `user_id`. Whether this is a new authenticated endpoint or an
    authenticated mode of the existing reserve is a cut-scope shape question.
  - `GET /api/v1/me/reservations` (or similar) — the reserver dashboard "things I've
    reserved" read (Decision 4).
  - **Admin invite / onboarding** (Decision 6) adds no new set-password machinery: the
    ADR-0009 create-by-email admin action emails the ADR-0011 single-use
    set-password/invite link. Whether "create user" triggers that send inline or via a
    distinct "send invite" admin action is a cut-scope shape question.
- **New storage:** an email-verification token table (migration, mirroring
  `password_reset_tokens`) and a pending/activation account state (Decision 3). Both
  additive.
- **Refines** ADR-0008 §5 (link-only becomes match-or-create-or-reject, but only when
  the policy opts in; `disabled` keeps link-only), ADR-0009 Cuts 2–3 (fills in their
  mechanics without changing the policy/role model), and composes with ADR-0007 §4
  (CAPTCHA on the low-trust email path) and ADR-0011 (empty-hash no-password state +
  the reset flow for password establishment + the token-store shape).
- **Unchanged and load-bearing:** the ADR-0009 policy values and role model; reserver
  anonymity to the owner (ADR-0002 §5); the co-buy reveal (ADR-0002 §6); the two-
  surface JWT/capability boundary and tenant-match invariant (ADR-0005 §2/§5); the
  anonymous `full_guest` / `email_confirmed` tiers (ADR-0007). Cross-tenant identity
  stays out of scope (accounts remain tenant-scoped, ADR-0009).
- **Open leans for sign-off** (collected): CAPTCHA on the email path only (Decision 1);
  empty-hash + reset flow for password establishment (Decision 2); the pending-account
  representation + verification-token TTL (Decision 3); the co-buy/decay interaction of
  a registered reservation and reset-style auto-login on verify (Decision 4); and the
  **unified invite/set-password onboarding vs. the emailed temp-password route
  (Decision 6 + Considered alternatives) — the operator's pick.** These are design
  choices to confirm at approval, not baked defaults.

## Rollout (proposed cuts, after sign-off — each its own PR, 2-of-2, spec-first)

Sequenced so each cut is independently useful and the trust-gated path lands before
the one-click path builds on the account model:

1. **Cut 1 — email + password signup (CAPTCHA + email verification) + unified invite.**
   The policy-gated `register` + `register/verify` endpoints, the email-verification
   token store (migration) and pending-account state, the CAPTCHA check on this path,
   and the signup + verify web pages. Folds in the Decision-6 onboarding: wire the
   ADR-0009 create-by-email admin action to send the ADR-0011 single-use
   set-password/invite link (reusing existing machinery), so admin-invited and
   self-registered accounts share one first-password path. Delivers self-registration
   for the low-trust path.
2. **Cut 2 — OAuth self-register + no-password reconciliation.** Extend the ADR-0008
   Google callback with the create-on-allowed-policy branch (Decision 1a), storing the
   empty-hash no-password state (Decision 2). No new callback endpoint; **resolves the
   Decision-2 `resettable()` reconciliation** (widen the guard, or add a distinct
   establish-password entry) so a no-password account can obtain a first-password link
   through the reused ADR-0011 token store.
3. **Cut 3 — registered-tier authenticated reserve + reserver dashboard.** The
   authenticated reserve path bound to `user_id` (no per-reservation email), the
   reserver "things I've reserved" dashboard, and the per-list `registered` option's
   decoupled warning (Decision 5). Makes ADR-0007's `registered` tier real end to end.

No code lands until this ADR is signed off. Cuts are sized here but each is reviewed
and gated on its own.
