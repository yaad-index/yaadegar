# ADR-0001: Foundations

**Status:** Accepted

## Context

Yaadegar is a self-hosted, open-source gift registry / wishlist. People who want
to give someone a gift pick from that person's list, without duplicate gifts and
without depending on a third-party SaaS. The name means *keepsake / memento*.

The product needs, from the outset, to support several non-trivial features:
reservations with anonymity, multiple lists per user, public share links,
one-click "add from a product URL", group co-buying of expensive items, stale-
reservation decay, optionally event-dated lists, and hosting for more than one
person (including friends who want it on their own domain). These shape the
foundational technical choices below.

## Decision

1. **API-first.** The core of the system is a full, external HTTP API that
   exposes every feature. All clients — the web app, any future mobile app, and
   automation — interact **only through this API**. No client accesses the
   database directly. The API surface is designed first (see the OpenAPI work),
   and the backend and clients are built against it.

2. **Backend in Go**, shipped as a single binary, consistent with the other
   projects in this fleet. Small operational surface, easy to self-host.

3. **Pluggable storage behind a repository interface.** Persistence is abstracted
   so the database is swappable, with two first-class drivers:
   - **SQLite** for local development and testing.
   - **Postgres** for real hosting, including multi-host / multi-tenant
     deployments and friends who self-host.
   The system is explicitly **not** SQLite-only. The storage interface and a
   migration strategy that covers both drivers are designed up front.

4. **Multi-tenant, Host-header-based routing from day one.** One instance serves
   many users. Routing resolves the incoming `Host` header to a tenant:
   - **Default:** each user gets a subdomain on the base host
     (e.g. `<username>.example.tld`), and that user's lists live under it.
   - **Bring-your-own-domain:** a user can point their own domain at the shared
     instance via CNAME, and that domain becomes the base URL for their lists.
   The API is host-aware — routes resolve to the actual tenant, not one central
   domain. Custom-domain **TLS** (on-demand ACME certificate provisioning plus a
   CNAME-verification step) is a later feature, **not** a day-one build, but the
   tenant model and routing are designed so it can be layered on **without a
   rewrite**. In short: ship single-tenant first, but build it multi-tenant-shaped.

5. **Frontend is a separate client.** The web UI is decoupled from the backend and
   consumes only the public API. It will be built **after** the API and backend,
   in **TypeScript** with a framework (Vue or similar; exact choice deferred). A
   mobile app is a possible later client of the same API.

6. **License: MIT. Public repository.** Open source from day one. Private hosting
   would hit CI limits and the project is intended to be open anyway.

## Consequences

- The API contract is the primary artifact and must be stable and well-specified;
  clients and backend both depend on it. This front-loads API design work.
- The repository/storage abstraction adds a little indirection but keeps the app
  portable across SQLite and Postgres, and keeps tests fast (SQLite) while
  production stays robust (Postgres).
- Building multi-tenant-shaped from the start (tenant resolution, per-tenant data
  scoping) is slightly more up-front design than a single-user app, but avoids a
  costly rewrite when custom domains land. TLS/ACME complexity is deferred, not
  designed out.
- Group co-buying deliberately breaks giver anonymity **only** through an explicit,
  opt-in, two-sided email handshake; the data model and API must encode consent
  before any contact is shared. (Detailed in a later ADR / the relevant issue.)
- Being public and MIT means all repository content (code, commit messages, ADRs,
  issues) must carry no personal identifiers; examples use generic placeholders.
