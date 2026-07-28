# ADR-0006: Frontend architecture

**Status:** Proposed

## Context

Yaadegar has been API-first from the start ([ADR-0002](0002-api-design.md)): a
documented HTTP surface with an owner side (`/api/v1`) and an anonymous public giver
side, tenant resolved from the request Host ([ADR-0004](0004-multi-tenant-routing-and-domains.md)),
and real owner authentication ([ADR-0005](0005-owner-authentication.md)). The
backend is feature-complete; this ADR fixes the web frontend (#11) that turns those
APIs into a usable product. It records the framework and the cross-cutting
principles; the phased feature scope is delivered in cuts on top of this.

## Decision

1. **Framework — SvelteKit + TypeScript.** The public share pages are
   **server-side rendered** so a shared wishlist link loads fast and carries proper
   OpenGraph/Twitter-card metadata for rich link previews (the share link is the
   product's viral surface). SvelteKit is proven, TypeScript-native, and has a
   light, low-dependency runtime footprint that fits the self-hosted, privacy-first
   ethos. TypeScript throughout.

2. **One app, route-segmented.** A single SvelteKit app serves both surfaces,
   sharing its client, components, and API layer: authenticated **owner** routes and
   the anonymous **public giver** routes (reached by a list's share slug, on the
   tenant's host). The tenant is resolved from the request Host — the backend already
   does Host → tenant, so the frontend forwards the Host and does not re-implement
   tenancy.

3. **Typed API client generated from the spec.** The API client types are
   **generated from `api/openapi.yaml`** (`openapi-typescript` for types +
   `openapi-fetch` for the typed request layer) and are **never hand-written**. A CI
   drift-guard regenerates and fails on a diff — the same lock-step the backend uses
   for its server code — so the frontend client can never silently diverge from the
   contract.

4. **Principle (hard): don't reinvent the wheel.** Use mature, proven
   libraries; never hand-roll what a well-maintained library already solves. For F1
   this fixes the baseline stack:
   - **UI/components:** a component library (Skeleton or shadcn-svelte) rather than
     bespoke primitives.
   - **Forms + validation:** `sveltekit-superforms` with `zod` schemas, not
     hand-written form state and validation.
   - **Data fetching/caching:** the TanStack Query Svelte adapter alongside
     SvelteKit `load` functions, not a bespoke cache.
   - **Session handling:** the owner session JWT (ADR-0005) is stored in an
     **httpOnly cookie** via SvelteKit server hooks — established session handling,
     not tokens in JS-readable storage.
   - **Auto-add-from-URL** calls the backend's existing SSRF-safe preview endpoint
     (#10); the browser never scrapes third-party pages itself.

5. **Deployment.** SvelteKit's **node adapter**, containerized and run alongside the
   backend (the existing compose/Dockerfile story extends to a `web` service). The
   frontend forwards the request Host so tenant routing continues to work end to end.

6. **Location.** The app lives under **`web/`** at the repo root, a self-contained
   Node project with its own `package.json`, lint/typecheck/build, and its generated
   client checked in and drift-guarded. The Go module is unaffected.

## Consequences

- The repo becomes a polyglot monorepo: the Go backend plus a `web/` Node app. Each
  has its own CI (the Go `check` job is unchanged; the web app adds lint + typecheck
  + build + client-drift verify).
- The OpenAPI spec is now the contract for **both** the generated Go server and the
  generated TypeScript client — a single source of truth, drift-guarded on both
  sides.
- Owner auth in the browser rides the JWT in an httpOnly cookie; the SvelteKit
  server layer attaches it as the `Authorization: Bearer` header when calling the
  backend, so the token is never exposed to client JS.
- **F1 (this cut) is foundations only** — framework, generated client, the library
  baseline with a trivial example, CI, and one placeholder page. **No owner or giver
  features.** Phase-1 features land in later cuts: owner login → create list → add
  items; giver views a shared link → reserve/release.
