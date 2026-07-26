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
