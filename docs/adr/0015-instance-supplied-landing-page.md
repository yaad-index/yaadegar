# ADR-0015: Instance-supplied landing page

**Status:** Proposed

## Context

An operator running their own instance may want a root page that is neither the
bundled marketing landing nor a redirect to login — their own wording, their own
framing, or a page for their family rather than for the public (#256). Today the
only outcomes are the shipped page or nothing.

Two premises in #256 do not hold against the current tree, and each changes what
this ADR has to decide.

**There is no on/off toggle to extend.** `web/src/routes/+page.server.ts` is the
whole of the root-route logic: an authenticated visitor is redirected to `/lists`
(which itself re-routes a giver onward), and everyone else gets the bundled
landing, unconditionally. There is no config key for it in `config.example.yaml`,
no env var, and nothing in the changelog for #236. So this ADR is not composing
with a boolean that already exists — it is deciding root-page behaviour from
scratch. That is the simpler position: one key that names *what is at the root*
is a cleaner extension point than a boolean plus an override that then has to
define what happens when the two disagree.

**"A config key in the same shape as the others" crosses a service boundary.**
#256 asks for a key shaped like the existing settings so "the sample config and
its drift guard cover it automatically." But `config.example.yaml` is the **Go
backend's** config, and per ADR-0014 the landing is served by the **web** service —
a separate image with its own environment-based configuration that never reads
that file. The guard is sharper than "covered automatically" implies: it walks the
backend CLI's `env:` tags and fails on any sample entry that is not one, in either
direction. So a web-intended key documented in `config.example.yaml` does not merely
get read by the wrong process — it **trips the guard as a stale entry** unless it is
made a genuine backend setting and fetched back over the API. The ask conceals the
service-ownership choice rather than settling it, and which service owns the setting
is therefore a real decision, not an implementation detail — one of the two the
sections below turn on.

The other is trust. #256 floats three shapes — a path to an HTML file served at
the root, a small set of overridable strings, a template directory that overrides
the built-in one — and lists the injection surface as one constraint among
several. It is the property that should drive the choice rather than trail it:
operator-supplied HTML at the site root is same-origin with an authenticated
session, and an extension point is hard to narrow once published (#256's own
caution).

## Decision

### 1. One key names what is at the root: `bundled` | `login` | `custom`

The root-page behaviour for a **signed-out** visitor is selected by a single
setting with three values:

- **`bundled`** (default) — the shipped marketing landing. Byte-for-byte today's
  behaviour.
- **`login`** — signed-out visitors are redirected to `/login`. This is the
  private-instance case that #256's "off means login" language gestured at: an
  operator whose instance has no public face at all.
- **`custom`** — the bundled layout with operator-supplied text (§2).

An instance that sets nothing is `bundled` and behaves exactly as it does today.
Authenticated routing is **unchanged and identical across all three values** — an
authenticated visitor still goes to `/lists`, and role-routing stays in one place
(`+page.server.ts`). This key decides only what a signed-out visitor sees.

A boolean-plus-override shape was rejected for the reason above: it has to answer
"what happens when the toggle says on but an override is also set," and an enum
that names the outcome directly never poses the question.

### 2. `custom` is overridable strings over the bundled layout, not operator markup

The `custom` value is realised as a fixed, named set of text slots the bundled
layout already contains — headline, sub-headline, the primary call-to-action's
label and destination, and the trust-strip line. Each is inserted as text or as
an attribute value through the framework's normal escaping, **never as raw HTML**.
The text slots therefore carry no injection surface. The one attribute slot — the
call-to-action's destination — is URL-valued, so it carries a *bounded* one:
attribute-escaping keeps a value from breaking out of the attribute, but a
`javascript:` or `data:` scheme is still a live vector when the link is clicked,
which the build closes with an allowlist — `http`/`https` and site-relative paths
only, everything else rejected. That is a small, closeable check on a single field,
categorically unlike the open-ended same-origin-authority surface that operator
markup opens (§3). The distinction the choice turns on is that difference in kind,
not a claim that strings have no surface whatsoever.

This covers what #256 is mostly asking for once "a page for their family" is
separated from "a second implementation of the page": the shipped design, the
operator's own words, their instance's framing. It is deliberately the **narrow**
extension point. Widening `custom` later — from strings to a richer, still-trusted
mechanism — is cheap; narrowing a published raw-HTML point is not.

The raw-HTML-file and template-directory shapes are **considered and not shipped
here**, for three reasons taken together:

- The injection surface they open (§3), which strings avoid entirely.
- The narrow-first rule above — an extension point is hard to take back.
- A fully bespoke root already has an operator-level answer that needs nothing
  first-party: a reverse proxy in front of the instance can serve its own `/` and
  pass every application route through. The bespoke-arbitrary-page need does not
  require an extension point in this codebase, whereas the string slots — the
  shipped layout with different words — cannot be had any other way.

This is a deferral, not a permanent refusal (§3 closes on when it could return).

### 3. Operator markup at the root is a same-origin, app-authority surface

Stated plainly, because "trusted because the operator wrote it" is a real decision
with a real blast radius rather than a formality:

The session is an httpOnly cookie (ADR-0006), so page script cannot read the token.
But any script that runs at the root runs **same-origin**, and its `fetch` calls to
`/api/v1` carry that cookie automatically — so markup at the root can act with the
authority of whatever session is signed in on that browser. httpOnly bounds token
*theft*; it does not bound credentialed same-origin *action*.

For self-hosted software the operator already owns the deployment, so treating
operator-authored root content as trusted is defensible — **but only for content
the operator genuinely authors**. The moment a third-party embed, an analytics
snippet, or a copied widget lands in that HTML, the trust is misplaced and the
blast radius is the operator's own signed-in users. Overridable strings carry none
of this, because a string is never markup.

If a first-party bespoke-page mechanism is later justified, a follow-up ADR adds it
as a **deliberately-trusted asset** with the boundary drawn at that point — a
Content-Security-Policy on the operator page, provenance rules, or an origin split —
rather than inheriting it silently from a file path. Widening §1's `custom` toward
that is additive; the reason it is not done now is that the surface is permanent
once shipped and the common need is met without it.

### 4. The setting lives on the web service, where the page is served

This is the central choice #256 forces, and it is presented as options because
either is defensible.

**Option A — a backend key, read by the web service over the API.** A landing
setting in `config.example.yaml` — a genuine backend key — which the web service
asks the backend for. Keeps a single config surface, and the sample-config guard
covers it correctly precisely because it is a real backend `env:` tag; it reads
like every other operator setting. Its costs are concrete: it adds a request on
the **root path** — the most-visited, least-authenticated page — and a failure
mode when the backend is unreachable, at which point the root either fails to
render its own shell or falls back to the bundled page anyway, reintroducing the
default it was trying to override.

**Option B — a web-service setting (recommended).** The landing setting is
environment configuration on the web image, alongside the operator env that image
already reads. Its cost, stated honestly, is that it gets **neither
`config.example.yaml` nor that file's drift guard** — and it must *not* be
documented there, since the guard rejects a sample entry with no backing backend
tag.

The recommendation is **B**, for reasons beyond the failure mode above:

- It **fits how the web service is already configured.** `ORIGIN`,
  `BACKEND_ORIGIN`, `PORT` and `HOST` are operator environment on the web image
  today; none of them lives in `config.example.yaml` and none is drift-guarded,
  and nobody treats that as a defect, because the web image is a separate config
  surface by ADR-0014. The landing setting is the same kind of thing. Option A's
  "one config surface" advantage is partly illusory — the web service already has
  its own surface, and A would keep this one key on the far side of it.
- The **root page stays backend-independent.** It renders from static config with
  no API call, so a backend outage cannot break the instance's front door.
- It is **proportionate.** A small set of static operator strings does not warrant
  a new backend endpoint, a startup fetch, and unreachable-fallback logic to keep
  one config file authoritative.

On the drift-guard gap, which is the real objection to B: the honest fix is to give
the web service its **own** documented config surface — a `web/.env.example` (or
equivalent) that lists these keys, and, if a guard is wanted, one that checks the
web service's own environment contract — not to move the setting into a process
that never serves the page so it can sit under the existing guard. A guard that
covers the key but is read by the wrong service is worse than an honest second
surface.

A third framing — a per-tenant, owner-editable landing stored in the database and
resolved through `settings.Resolve` — was set aside, not because it is wrong but
because it is a **different and larger** feature. #256 is an operator/deployment
concern ("an operator running their own instance"), phrased as a config key, not an
in-app owner setting; a DB-backed, per-tenant, UI-editable landing is its own ADR
if the need is shown.

Concretely, the web service reads (all optional; unset means the bundled string, so
`custom` with nothing set equals `bundled`):

- `YAADEGAR_ROOT_PAGE` = `bundled` (default) | `login` | `custom`
- `YAADEGAR_ROOT_HEADLINE`, `YAADEGAR_ROOT_SUBHEAD`,
  `YAADEGAR_ROOT_CTA_LABEL`, `YAADEGAR_ROOT_CTA_HREF`,
  `YAADEGAR_ROOT_TRUST_LINE` — the `custom` text slots.

The `YAADEGAR_` prefix keeps these in one recognizable namespace with the backend's
env; `ORIGIN`/`BACKEND_ORIGIN` keep their names because they are SvelteKit adapter
conventions, whereas these keys are the project's own.

### 5. The backend and the API surface are untouched

`config.example.yaml` is deliberately **not** modified — a landing key there is the
exact wrong-process mistake premise-2 would produce. No new backend endpoint and no
new API field are introduced, so ADR-0014's topology (the web service is the public
edge, the backend port is unpublished) is unchanged, and the root path makes no
cross-service call.

## Consequences

- An instance can present its own words at the root with the shipped design, send
  signed-out visitors straight to login, or keep the bundled page — with the
  bundled page the default, so an instance that sets nothing is unchanged.
- The bespoke-arbitrary-HTML need is **not** met first-party. Operators with that
  need use a reverse proxy today, and a later ADR can add a deliberately-trusted
  asset mechanism if the need is demonstrated. This is deliberate under the
  narrow-first rule, and the cost is borne by the least-common need.
- **No open-ended injection surface is created.** The text slots are text, not
  markup; the one URL-valued slot takes a bounded scheme allowlist (§2), not the
  same-origin-authority trust decision a raw-HTML path would force.
- The web service gains its first *operator-facing feature* configuration, beyond
  transport wiring. That argues for a documented web-config surface
  (`web/.env.example` or equivalent) the web image does not have yet — a small new
  maintenance item, and the right place to also record its existing
  `ORIGIN`/`BACKEND_ORIGIN` keys.
- The **drift-guard asymmetry is made explicit rather than papered over**: the web
  service's configuration is not covered by the backend's sample-config guard —
  here, and already for `ORIGIN`/`BACKEND_ORIGIN`. If that gap is worth closing it
  is closed for the whole web surface, not smuggled shut for this one key by
  putting it on the wrong service.
- The root page keeps its independence from backend availability: it had no
  cross-service dependency before and gains none.
- Extending `custom` later, from strings toward a richer still-trusted mechanism,
  is a widening. Nothing decided here has to be narrowed later.
