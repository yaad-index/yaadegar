# ADR-0004: Multi-tenant Host routing and custom domains

**Status:** Accepted

## Context

[ADR-0001](0001-foundations.md) fixed multi-tenant, Host-header-based routing from
day one: a default per-user subdomain plus bring-your-own custom domains, with
custom-domain TLS deferred. The routing itself (Host → tenant) shipped with the
storage layer and handlers (#4/#5). This ADR records the concrete multi-tenant
model — custom-domain verification, the security invariant that governs it,
hostname uniqueness, subdomain rules, and the explicit TLS boundary.

## Decision

1. **Routing.** The tenant is resolved from the request Host before routing. A
   host under the configured base domain resolves by its leftmost subdomain label
   (`TenantBySubdomain`); anything else is treated as a custom domain
   (`TenantByCustomDomain`). Host-string parsing lives in the API layer; storage
   holds no base-domain policy.

2. **Custom-domain verification — DNS TXT-token challenge.** `addDomain`
   registers an **unverified** hostname, mints a stable per-domain
   `verification_token`, and returns the CNAME target to point at. The owner adds
   a DNS TXT record `_yaadegar-verify.<hostname>` with the token as its value;
   `verifyDomain` performs a TXT lookup (via an injectable resolver, with a
   timeout) and marks the domain verified on a match. A TXT challenge proves
   control unambiguously and independently of CNAME propagation.
   - **Idempotent.** Re-verifying an already-verified domain is a clean success.
     A missing or non-matching TXT record — including NXDOMAIN or a lookup
     timeout — is a *normal not-yet-verified* outcome the owner retries, never a
     server error.
   - The `verification_token` is a **proof-of-control challenge, not a secret
     capability**: it is fine to store in plaintext and expose, and it is stable
     so re-verification works. It must not be confused with the one-time
     capability tokens (reservations/co-buying), which are hashed and never
     re-shown.

3. **Security invariant — verified-only resolves.** `TenantByCustomDomain`
   resolves **only** `verified = true` domains. An unverified hostname — including
   another tenant's parked, unverified claim — never routes. This is the single
   load-bearing check for custom-domain tenancy and is pinned by a regression
   test.

4. **Hostname uniqueness — first-add-wins (v1).** `domains.hostname` is globally
   unique, so the first tenant to *add* a hostname claims it. This is fully secure
   on the resolve path: an unverified squatter can never route, because they
   cannot control the domain's DNS. The only downside is a low-severity add-time
   namespace DoS — an unverified claim can block a legitimate owner from
   registering a hostname a squatter has parked. This is acceptable for v1
   (self-hosted; the operator controls tenant creation). The future mitigation —
   expiring unverified claims after N days — is tracked in #42 and referenced here
   rather than left as prose. `first-verified-wins` (allow multiple unverified
   claims, unique only among verified) was considered but rejected for v1: it
   requires dropping the unique constraint (a SQLite table rebuild) for marginal
   benefit given no routing impact.

5. **Subdomain assignment.** A tenant's subdomain must be set, slug-formatted
   (lowercase alphanumeric and hyphens, bounded length), unique (enforced by the
   DB), and not reserved. Reserved names are an **instance-config denylist**
   (default `www`, `api`, `app`, `admin`) so operators can extend it; the base
   domain itself is not a claimable subdomain.

6. **TLS is deferred.** `tls_status` is a placeholder (`none`/`pending`); no ACME
   or on-demand certificate provisioning is built in this issue. The model is
   deliberately multi-tenant-shaped so on-demand TLS (a CNAME-verified ACME flow)
   can layer on later without a rewrite. The OpenAPI note about on-demand TLS on
   `addDomain` is aspirational until that lands.

7. **Reverse-proxy trust — `X-Forwarded-Host`, off by default.** When the backend
   runs behind the trusted frontend (the SvelteKit server proxies owner-surface
   calls, ADR-0006), the original tenant host must reach the backend, but Node's
   `fetch` forbids overriding the outbound `Host` header. The standard reverse-proxy
   answer is `X-Forwarded-Host`, gated by an explicit, **default-off**
   `trust_forwarded_host` setting:
   - **Off (default).** The tenant resolves from `Host` exactly as before;
     `X-Forwarded-Host` is ignored entirely. This is the untrusted-safe default:
     the header is client-settable, so honoring it on a directly-reachable backend
     would let anyone spoof any tenant.
   - **On.** `X-Forwarded-Host` takes precedence, with `Host` as the fallback when
     it is absent. Enable it **only** when the backend port is not externally
     reachable and requests arrive exclusively through the trusted frontend (e.g. an
     unpublished backend on a compose network). This mirrors the well-trodden
     `USE_X_FORWARDED_HOST` pattern.
   - **Load-bearing invariant:** with trust off, `X-Forwarded-Host` never influences
     tenant routing — pinned by a test.
   - **Future hardening (noted, not built for v1):** if the backend is ever exposed
     directly while still fronted by the proxy, add a shared-secret header between
     the frontend and backend so a forwarded host is trusted only when accompanied
     by the secret. Recorded here so it is not re-invented.

## Consequences

- Custom domains are self-service — add → prove control via TXT → verified →
  routes — with no operator step, but serving them over HTTPS is a later feature.
- The verified-only invariant is the one security-critical check and is tested
  directly (unverified does not resolve; verified does).
- first-add-wins keeps the schema unchanged now; #42 tracks the squatting
  mitigation so the tradeoff is real backlog.
- The injectable resolver keeps verification fully testable with no real DNS.
