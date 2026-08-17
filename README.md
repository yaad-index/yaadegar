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

Bring up the whole app — web frontend, backend, and Postgres — with Docker:

```sh
docker compose up --build
```

The backend applies its migrations on start and runs on the internal compose
network (its port is not published); the web UI is served at
`http://localhost:3000`. Tenants are addressed by subdomain under `localhost`
(e.g. `alice.localhost`), which resolves to the loopback with no `/etc/hosts`
changes — so an owner logs in at `http://alice.localhost:3000`.

Owner self-registration isn't built yet, so seed a tenant and an owner from the
CLI (the same binary, inside the running backend container):

```sh
docker compose exec app yaadegar create-tenant --subdomain alice
docker compose exec app yaadegar create-owner --tenant alice --username alice --password devpass
```

Then open `http://alice.localhost:3000`, log in as `alice`, create a list, and
copy its share link to try the anonymous giver flow.

To stand it up on a remote demo host (a LAN box with no TLS or DNS), override the
origin, base domain, and published port at up-time — a nip.io host needs no DNS
setup:

```sh
YAADEGAR_BASE_DOMAIN=203.0.113.10.nip.io \
  ORIGIN=http://demo.203.0.113.10.nip.io:3000 \
  WEB_PORT=3000 docker compose up --build --detach
docker compose exec app yaadegar create-tenant --subdomain demo
```

`ORIGIN` must match the host the browser uses (it is the frontend's CSRF origin).
Cookies are not marked `Secure` over plain http — fine for a LAN demo; a real TLS
deployment fronts the web service with a terminating proxy (see ADR-0006 §5).

> **Dev only.** The compose file ships intentionally weak, placeholder secrets for
> local use. Never reuse them for anything real.

## Container image

Prebuilt multi-arch images (`linux/amd64`, `linux/arm64`) are published to GHCR:

```sh
docker pull ghcr.io/yaad-index/yaadegar:latest
```

Images are published on each release (`vX.Y.Z`, `X.Y`, and `latest`) and on every
push to `main` (a rolling `main` tag plus a short-sha tag for bleeding-edge
testing). To use one with the local compose setup, set the `app` service's `image:`
to the tag you want instead of `build: .`.

## Self-hosting

Standing up a real instance — production config, PostgreSQL, TLS and reverse-proxy
wiring, multi-tenant subdomains and custom domains, bootstrapping the first
tenant/owner/admin, Google sign-in, and health checks — is covered end to end in
the [self-hosting guide](docs/self-hosting.md).

## Import / export

Owners can export a list's **item catalog** for backup or portability, and (soon)
re-import it. The export is deliberately **identity-free**: it never includes who
reserved, reservation state, availability, funded amounts, ids, or timestamps —
only the fields you authored.

Download from a list's page, or directly:

```
GET /api/v1/lists/{listId}/export?format=json   # default
GET /api/v1/lists/{listId}/export?format=csv
```

**JSON** is a versioned envelope — the stable, round-trippable contract:

```json
{
  "schema_version": 1,
  "items": [
    {
      "name": "Espresso machine",
      "url": "https://example.com/p/123",
      "image_url": null,
      "price_amount_minor": 24900,
      "price_currency": "EUR",
      "note": "the white one",
      "priority": 0,
      "quantity_wanted": 1,
      "allow_cobuy": null,
      "thank_you_template": null
    }
  ]
}
```

`price_amount_minor` + `price_currency` are set together or both absent.
`allow_cobuy` and `thank_you_template` are the **raw per-item overrides** —
`null` means "inherit the list default", so a re-import preserves your inherit
semantics rather than baking in resolved values.

**CSV** carries the same fields as fixed columns (RFC 4180):

```
name,url,image_url,price_amount_minor,price_currency,note,priority,quantity_wanted,allow_cobuy,thank_you_template
```

CSV cannot distinguish `null` from an empty string, so on a CSV re-import an empty
`allow_cobuy` / `thank_you_template` reads as *inherit*; use the JSON form if you
need to preserve an explicit empty (opt-out) value exactly.

## Design

The interface — screens, palette and typography — is designed by [@mahboub8061](https://github.com/mahboub8061).

## Developed by AI

Yaadegar is designed, built, and reviewed by AI agents, part of an AI-run open-source org. Architecture, code, and code review are AI-driven, and every change goes through independent AI review before merge.

## License

MIT.
