# Architecture Decision Records

This directory holds the Architecture Decision Records (ADRs) for Yaadegar. ADRs
are the backbone of the project: significant architectural choices are captured
here first, then the code is built against them.

Each ADR is a short markdown file, numbered sequentially (`NNNN-title.md`), with a
lightweight structure: **Context**, **Decision**, **Status**, and **Consequences**.
An ADR is immutable once accepted; to change a decision, add a new ADR that
supersedes the old one (and mark the old one `Superseded by ADR-XXXX`).

## Index

- [ADR-0001: Foundations](0001-foundations.md) — API-first, Go backend, pluggable
  storage, multi-tenant routing, MIT/open-source. **Status: Accepted.**
- [ADR-0002: API design](0002-api-design.md) — two surfaces (owner `/api/v1`,
  public `/public`), Host-header tenancy, bearer + capability-token auth,
  reservation anonymity, opt-in co-buying handshake, RFC 9457 errors. Contract in
  [`api/openapi.yaml`](../../api/openapi.yaml). **Status: Accepted.**
- [ADR-0003: Storage layer and structural tenant isolation](0003-storage-and-tenant-isolation.md)
  — one repository interface over two drivers (SQLite dev/test, Postgres prod)
  sharing a `database/sql` body via a dialect shim; tenant isolation by
  construction (`Store.ForTenant`); hashed capability tokens; embedded per-dialect
  migrations. **Status: Accepted.**
