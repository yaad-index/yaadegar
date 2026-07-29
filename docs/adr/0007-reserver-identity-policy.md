# ADR-0007: Reserver-identity policy

**Status:** Proposed

## Context

The public giver surface (ADR-0002 §5) lets anyone reserve an item with an anonymous one-time capability token (implemented in the F3 giver flow). This is the lowest-friction tier and also the most abusable: a bot can reserve items purely to grief availability, and the system has no verified contact for the reserver unless they volunteer one, so reservation-decay reminders and a future "items I've reserved" cross-list view have nothing reliable to key on.

ADR-0005 §6 named three guest tiers — `full_guest` / `email_confirmed` / `registered` — but deferred the policy. This ADR is that policy (#19). The anti-bot CAPTCHA (#45) depends on it: CAPTCHA must know which reserve paths are "low-trust."

One invariant governs the whole design and does not move: **the reserver stays anonymous to the list owner at every tier** (ADR-0002 §5). Identity tiers change only what the *system* knows (a verified contact, or an account), never what the owner or other givers can see.

## Decision

### 1. Three trust tiers — per-list, with an instance default

- **`full_guest`** — capability token + optional display name. Lowest friction, no verified contact. This is the current F3 behavior.
- **`email_confirmed`** — the giver supplies an email and confirms it via a one-time link before the reservation becomes active. Gives the system a verified contact (decay reminders; the future cross-list "things I've reserved" view keyed off the confirmed email), still anonymous to the owner.
- **`registered`** — a giver account is required (authenticated giver session). Strongest identity, lowest abuse. Depends on a giver-account system that does not exist yet; **specified here, implemented later** (honest deferral).

Configuration reuses the existing `settings.Resolve[T]` nullable-override pattern (as reservation-decay does): an **instance default** plus an optional **per-list override**. The domain exposes an effective tier; a nil per-list value inherits the instance default.

### 2. Anonymity invariant (reaffirmed, unchanged)

At no tier does the owner — or any other giver — learn who reserved. `email_confirmed` and `registered` give the *system* a contact/identity for reminders, abuse control, and the reserver's own view; that data is never surfaced on the owner or public surfaces. The existing rule holds: owner/other-giver responses emit no reserver identity, and the tier setting does not relax it.

### 3. Interaction with the capability-token flow

- **`full_guest`** — unchanged. Reserve returns the one-time capability token immediately; the reservation is created active.
- **`email_confirmed`** — reserve creates a **`pending_confirmation`** reservation and emails a one-time confirmation link. The capability token is activated only on confirmation. The pending reservation **holds the item provisionally** (so a confirming giver is not beaten to the last unit), and **auto-expires** if not confirmed within a short confirm-window, reusing the decay engine's injected clock + settings. Rationale for the provisional hold: it protects the good-faith giver's UX; the griefing risk it introduces (a bot reserving-without-confirming to tie up items) is bounded by the short window and gated by the #45 CAPTCHA on this path. The confirm-window **defaults to ~30 minutes (configurable)** — long enough for slow email delivery, short enough to bound griefing dwell time. *Considered alternative:* no hold until confirmed (item stays available; oversell handled by the atomic capacity check at confirm time). Rejected for v1 because it turns the confirm step into a race — the giver clicks Reserve, checks email, and returns to find the item gone — which reads as broken; revisit if griefing proves real.
- **Availability during the pending window.** A `pending_confirmation` reservation counts as reserved on **both** the owner and public surfaces (shown as reserved / not available), so the owner does not separately arrange the gift and other givers do not race for it. The reserver's identity and email stay hidden from the owner throughout, per §2 — the item reads as "reserved," never by whom.
- **`registered`** — reserve requires a valid giver session; the account is the release capability (per-item token still works but is not required).

### 4. Low-trust paths — the definition #45 consumes

- **Low-trust = `full_guest` reserve + `email_confirmed`'s initial (pre-confirmation) reserve.** These are the paths the #45 CAPTCHA gates: the CAPTCHA token is verified server-side before the (active or pending) reservation is created.
- **`registered` is not low-trust** — an authenticated giver skips the CAPTCHA.

### 5. Reservation state-machine impact

- Adds `pending_confirmation` to the lifecycle for `email_confirmed`: `pending_confirmation → active` (on confirm) or `→ expired` (confirm-window elapses). This composes with the existing decay states (`active → reserver_notified → expired`); confirm-expiry is a distinct, earlier gate with its own window.
- `full_guest` reservations skip `pending_confirmation` (created `active`), as today.

### 6. What ships when

- **v1 (first cut of this ADR):** `full_guest` (exists) + `email_confirmed`. `registered` is deferred until a giver-account system exists.
- **Instance default:** `full_guest` (lowest friction) for the default self-host, with `email_confirmed` available per-list for owners who want less abuse. Operators tune per audience; the per-list override lets a high-value list demand more.

## Consequences

- Owners get an abuse-vs-friction dial without ever seeing who reserved.
- #45 (CAPTCHA) has an unambiguous definition of the low-trust paths to gate.
- `email_confirmed` introduces a `pending_confirmation` lifecycle and a confirm-window sweeper — reuse the decay infrastructure (clock, settings, sweeper) rather than a parallel mechanism.
- `registered` is specified but not built; the ADR is honest that it awaits a giver-account system (which the cross-list "things I've reserved" view also needs).
- The anonymity invariant is untouched: more identity to the system, never to the owner.
