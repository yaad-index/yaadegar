# ADR-0013: Anti-bot CAPTCHA on low-trust reserve (pluggable verifier)

**Status:** Accepted

## Context

The public giver surface lets anyone reserve an item. The two low-trust reserve tiers (ADR-0007) — `full_guest` (anonymous capability token) and `email_confirmed` (unverified email until a confirm click) — are abusable by bots that reserve items purely to grief availability. ADR-0007 named the anti-bot CAPTCHA (#45) as its dependent follow-up: "CAPTCHA must know which reserve paths are low-trust." That policy is now implemented and closed (#19), so the low-trust paths are well-defined.

The code already anticipates this: `reservations.go` carries a deliberate `captchaGate` no-op seam, called today only from the `email_confirmed` path, documented as "the seam for the #45 low-trust pre-confirm CAPTCHA … so the check has a single, obvious place to land later." This ADR fills that seam.

Design constraints from the owner (issue #45): simple image CAPTCHAs are trivially solved by modern bots and are inadequate — use a modern managed/behavioral CAPTCHA (reCAPTCHA-class). It must be **pluggable** (operator picks the provider and supplies their own keys, no hard-wired single vendor) to fit the self-hosted, privacy-first model. Verify the token server-side before the reservation is created; a missing/invalid token is refused. Authenticated (`registered`) reservers skip it.

## Decision

### 1. A pluggable `captcha.Verifier` interface

Mirror the existing `email.Sender` extension-point pattern (interface + nil-default, injected via `api.Options`):

```go
package captcha
// Verifier checks a client-supplied CAPTCHA response token server-side. A nil
// error means "human / passed"; any error means "refuse the request".
type Verifier interface {
    Verify(ctx context.Context, token, remoteIP string) error
}
```

- Concrete implementations for the **managed-token providers** — all of which share the same "client widget yields a response token, server POSTs token+secret to the provider's verify endpoint" shape: **Cloudflare Turnstile** (default recommendation — privacy-friendly, free, no Google-account lock-in, and the operator already runs Cloudflare), **hCaptcha**, and **Google reCAPTCHA** (v2/v3).
- The nil default is a **`NoopVerifier`** that returns nil (CAPTCHA disabled). An instance with no CAPTCHA configured behaves exactly as today — this is backward-compatible and keeps CAPTCHA strictly opt-in.

### 2. Where it gates — the two low-trust paths only

- **`full_guest`** and **`email_confirmed`**: the verifier runs **before** the reservation is created (before `CreateWithinCapacity` / the pending-confirmation insert). Generalize the existing `captchaGate` seam to take the request's CAPTCHA token + client IP and call `s.captcha.Verify(...)`; wire it into the `full_guest` path too (today it is only called from `email_confirmed`).
- **Absent token short-circuits.** When CAPTCHA is required (low-trust tier + a configured verifier) and `captcha_token` is absent/empty, refuse with **400 before calling `Verify`** — never pass an empty string to the provider. `Verify` is only invoked with a non-empty token.
- **`registered`**: skipped. The reserver is already authenticated, and the anonymous surface already 401s a registered-tier list. No CAPTCHA on the authenticated `/me/reservations` path.
- Refusal → **400** RFC-9457 problem (`captcha verification required/failed`). No enumeration concern here (the reserve surface is public by design).

### 3. Token transport — an optional request-body field

Add an optional `captcha_token` string to the `CreateReservation` request body in `api/openapi.yaml` (regenerate the server + TS client through the existing drift guard). Optional at the schema level (so `registered`/disabled instances need not send it); **enforced server-side** when the list's effective tier is low-trust AND a verifier is configured. The client IP passed to `Verify` (providers accept an optional `remoteip`) is sourced consistently with the existing proxy-trust model (`TrustForwardedHost` → `X-Forwarded-For`, else `RemoteAddr`).

### 4. Instance configuration (operator-supplied)

Wired at startup into the concrete verifier and injected via `Options.Captcha`, mirroring the OAuth/email config wiring:

- `YAADEGAR_CAPTCHA_PROVIDER` = `turnstile` | `hcaptcha` | `recaptcha` | `none` (default `none` → `NoopVerifier`, disabled).
- `YAADEGAR_CAPTCHA_SECRET` — the provider verify secret (server-side, never exposed).
- `YAADEGAR_CAPTCHA_SITE_KEY` — the public site key (safe to expose to the frontend).

An unknown/absent provider fails closed at **startup config validation** (like the fail-closed auth construction), not silently — except `none`, which is the explicit disabled state.

### 5. Frontend widget

The reserve form renders the configured provider's widget using the public site key, obtained from the existing public instance-config surface (the same mechanism that exposes the OAuth-enabled flag and the effective reserver tier to the giver page). The public config exposes **both `captcha_provider` and `captcha_site_key`** — the frontend loads a different JS SDK per provider (Turnstile vs hCaptcha vs reCAPTCHA), so the provider name is needed, not just the key. The form includes the resulting token in the reserve request. The widget shows only when CAPTCHA is enabled AND the list's effective tier is low-trust. The `registered` path is unaffected.

### 6. Provider outage → fail-closed (bounded)

When a verifier is configured, a provider timeout/error on the verify call **refuses the reserve** (fail-closed) with a bounded timeout of **5 seconds** (fixed in the ADR so implementors don't each pick their own), logged. Rationale: the entire point is anti-abuse; a provider outage must not silently open the low-trust paths. The tradeoff (provider downtime blocks legitimate low-trust reserves) is acceptable because reserve is not time-critical and the operator opted in; the escape hatch is to set `PROVIDER=none`. Registered reservers are never affected.

## Consequences

- Adds an external HTTP dependency on the reserve path **only when enabled**; disabled instances are byte-for-byte unchanged (NoopVerifier).
- OpenAPI change → regenerated server + TS client, guarded by the existing CI drift check. This is a backend + web + spec change (not backend-only), so it needs a runtime UI test of the widget + reserve flow, not just a code read.
- Privacy/self-hosting preserved: operator picks the provider and holds the keys; no vendor is hard-wired. A zero-third-party option (Altcha proof-of-work) is a planned follow-up (see Rollout cut 2).
- The anonymity invariant (ADR-0007 §2) is untouched: CAPTCHA gates bot abuse; it reveals nothing about the reserver to the owner or other givers.

## Rollout (proposed implementation cuts)

- **Cut 1 — the subsystem + managed providers.** `captcha.Verifier` interface + `NoopVerifier` + Turnstile (default) + hCaptcha + reCAPTCHA; `Options.Captcha` wiring + config validation; generalize the `captchaGate` seam and wire both low-trust paths; `captcha_token` in the OpenAPI reserve request; the frontend widget + public-config exposure of provider/site-key. Ships the full operator value.
- **Cut 2 — Altcha proof-of-work (follow-up).** A self-hostable, zero-third-party PoW option. Deferred because its model differs from the managed providers: it needs a server-issued challenge endpoint (GET challenge → client solves → POST solution), which doesn't fit the single `Verify(token)` call cleanly and is best designed as its own small extension (a challenge issuer + an Altcha verifier) once cut 1 lands.

Per-list CAPTCHA override is intentionally out of scope: CAPTCHA is an instance-level abuse control, keyed off the already-per-list effective tier.
