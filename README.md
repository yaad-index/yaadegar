# Yaadegar

**Yaadegar** (یادگار, "keepsake") is an API-first, self-hosted gift registry and wishlist.

Make a list of things you would like to receive, share a link, and let friends and family reserve items without stepping on each other — all on infrastructure you own.

## Why Yaadegar

- **Reserver anonymity by default.** The list owner sees that an item is reserved, never who reserved it, so gifts stay a surprise. Enforced server-side, not just hidden in the UI.
- **Group co-buying.** For an expensive item, friends can split the cost through an opt-in, two-sided email handshake, and only see each other's contact once both agree.
- **Reservation decay.** Stale reservations check back with the reserver and free themselves if abandoned, so an item is never locked forever.
- **Event-dated lists.** A list for a specific occasion auto-closes after its date; evergreen lists live forever.
- **Browser auto-add.** Paste a product URL and Yaadegar drafts the item from the page's metadata, with a strict, SSRF-safe fetch.
- **Multi-tenant.** One instance serves many people, each with their own subdomain, and anyone can bring their own custom domain.
- **API-first.** Every capability is a documented HTTP API (OpenAPI 3.1); the spec is the source of truth and clients never touch the database directly.

## Status

The backend is feature-complete and covered by tests. A web frontend is in progress. Real owner authentication and TLS for custom domains are on the near-term roadmap.

## Tech

- Go backend, single binary.
- Pluggable storage: SQLite for development, PostgreSQL for production, behind one repository interface with structural per-tenant isolation.
- OpenAPI-as-source-of-truth via oapi-codegen, with a CI drift guard.

## Run it locally

Bring up the app and a Postgres database with Docker:

```sh
docker compose up --build
```

The app applies its migrations on start and listens on `http://localhost:8080`.
Tenants are addressed by subdomain under `localhost` (e.g. `alice.localhost`),
which resolves to the loopback with no `/etc/hosts` changes.

Owner self-registration isn't built yet, so seed a tenant and an owner from the
CLI (the same binary):

```sh
docker compose run --rm app create-tenant --subdomain alice
docker compose run --rm app create-owner --tenant alice --username alice --password devpass
```

Then log in to get a session token and use it on the owner surface:

```sh
curl -sX POST http://alice.localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"alice","password":"devpass"}'
# → {"access_token":"<jwt>","token_type":"Bearer","expires_in":43200}

curl -s http://alice.localhost:8080/api/v1/me -H 'Authorization: Bearer <jwt>'
```

> **Dev only.** The compose file ships intentionally weak, placeholder secrets for
> local use. Never reuse them for anything real.

## Developed by AI

Yaadegar is designed, built, and reviewed by AI agents, part of an AI-run open-source org. Architecture, code, and code review are AI-driven, and every change goes through independent AI review before merge.

## License

MIT.
