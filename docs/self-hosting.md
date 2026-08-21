# Self-hosting Yaadegar

An end-to-end guide to standing up your own Yaadegar instance — from a first local
run to a TLS-terminated, multi-tenant production deployment with Google sign-in.

Yaadegar is API-first and self-hosted: you run the whole thing on infrastructure
you own. This guide uses **placeholders** throughout (`example.com`,
`<your-domain>`, `<client-id>`, …); substitute your own values.

- [Architecture at a glance](#architecture-at-a-glance)
- [Prerequisites](#prerequisites)
- [Deploy with Docker Compose](#deploy-with-docker-compose)
- [Configuration](#configuration)
- [Bootstrap the first tenant, owner, and admin](#bootstrap-the-first-tenant-owner-and-admin)
- [Reverse proxy, custom domains, and TLS](#reverse-proxy-custom-domains-and-tls)
- [Google OAuth setup](#google-oauth-setup)
- [Verify a healthy instance](#verify-a-healthy-instance)
- [Upgrades and backups](#upgrades-and-backups)

---

## Architecture at a glance

A Yaadegar deployment is three services:

| Service | What it is | Exposed? |
| --- | --- | --- |
| `app` | The Go API server (the source of truth; single binary). | **No** — internal only. |
| `web` | The SvelteKit frontend (reference UI + the public API door). | Yes — the one published port. |
| `db`  | PostgreSQL (production) or an embedded SQLite file (dev). | No. |

The **`web` origin is the only public surface.** It serves the UI and also
transparently proxies the versioned REST API at `/api/v1/*` through to `app`, so
external clients (mobile, `curl`, third-party frontends) reach the real API on the
same origin. The `app` port is deliberately **never published** — the frontend is
its sole trusted proxy, which is what makes it safe to resolve the tenant from the
forwarded host (see [Configuration](#configuration) and ADR-0004 §7).

Tenants are addressed by **subdomain** under a base domain you choose
(`alice.example.com`, `bob.example.com`), and an owner may additionally bring their
own **custom domain**.

## Prerequisites

- **Docker + Docker Compose** (the supported path), or the Go binary plus your own
  PostgreSQL if you prefer to run it directly.
- For anything beyond a LAN demo: a **domain you control** and the ability to set
  **wildcard DNS** (`*.example.com`) so tenant subdomains resolve, plus a
  **TLS-terminating reverse proxy** (nginx, Caddy, Traefik, a cloud LB, …).
- **Optional — outgoing email:** an SMTP relay. Reservation-confirm and
  reservation-decay emails only send when SMTP is configured; otherwise they are
  logged. Email is required for the `email_confirmed` reserver tier and for
  co-buying handshakes to actually reach givers.
- **Optional — Google sign-in:** a Google Cloud project (see
  [Google OAuth setup](#google-oauth-setup)).

## Deploy with Docker Compose

The repository ships a `compose.yaml` that brings up all three services. The
backend applies its database migrations automatically on start.

```sh
docker compose up --build
```

The web UI is served at `http://localhost:3000`. Tenant subdomains under
`localhost` resolve to the loopback with no `/etc/hosts` edits, so a local owner
logs in at `http://alice.localhost:3000`.

> **The bundled compose secrets are DEV ONLY.** `compose.yaml` ships intentionally
> weak, placeholder secrets (JWT signing secret, Postgres password) for local use.
> **Never** reuse them for a real deployment — see [Configuration](#configuration)
> for the values you must override.

### Deploy from the published images (no source checkout)

Two multi-arch images (`linux/amd64`, `linux/arm64`) are published to GHCR as a
**matched pair** on each release — the API (`ghcr.io/yaad-index/yaadegar`) and the
web frontend (`ghcr.io/yaad-index/yaadegar-web`). Both carry the same version tag
for a release, so pinning one version pins a coherent pair (ADR-0014). An
images-only deployment that ran the API alone would start healthy and serve no
site, so the frontend is published alongside it rather than left to a source build.

[`docs/docker-compose.yml`](docker-compose.yml) brings up all three services from
those published images — a source-free quick start:

```sh
docker compose -f docs/docker-compose.yml up -d
```

Pin the release with `YAADEGAR_IMAGE_TAG` (both images move together); it defaults
to a pinned version in the file:

```sh
YAADEGAR_IMAGE_TAG=0.13.0 docker compose -f docs/docker-compose.yml up -d
```

> `YAADEGAR_IMAGE_TAG` selects the image tag only. Do **not** rename it to
> `YAADEGAR_VERSION` — that is the web image's own build stamp, which the
> frontend/backend version-skew check reads; overriding it would make that check
> report the value you set instead of the image's real version.

Seed the first tenant and owner through that same compose (see
[Bootstrap](#bootstrap-the-first-tenant-owner-and-admin) for the full sequence):

```sh
docker compose -f docs/docker-compose.yml exec app yaadegar create-tenant --subdomain alice
```

As with the bundled dev compose, the `app` backend port stays **unpublished** —
`web` is its sole trusted proxy (ADR-0004 §7) — and the shipped secrets are
DEV-ONLY placeholders you must override for anything real (see
[Configuration](#configuration)).

## Configuration

Every setting resolves through three layers, **lowest to highest precedence**:

```
config file  <  environment variable  <  command-line flag
```

The config file is an optional YAML file searched at `/etc/yaadegar/config.yaml`
then `./config.yaml`. Copy [`config.example.yaml`](../config.example.yaml) — it
documents every key with its matching env var and flag — and edit what you need.

**Secrets are read from the environment only** and must never be written into the
config file: the JWT signing secret, the SMTP password, and the Google OAuth
client secret.

### Core settings

| Setting | Env var | Notes |
| --- | --- | --- |
| Base domain | `YAADEGAR_BASE_DOMAIN` | Host suffix tenant subdomains live under, e.g. `example.com`. Hosts outside it are treated as custom domains. |
| Storage driver | `YAADEGAR_STORAGE_DRIVER` | `sqlite` (dev) or `postgres` (production). |
| Storage DSN | `YAADEGAR_STORAGE_DSN` | A SQLite file path/URI, or a Postgres URL like `postgres://user:pass@host:5432/yaadegar?sslmode=require`. |
| Listen address | `YAADEGAR_HTTP_ADDR` | Defaults to `:8080`. |
| Trust forwarded host | `YAADEGAR_TRUST_FORWARDED_HOST` | Resolve the tenant from `X-Forwarded-Host`. Turn **on** in the compose/proxy deployment (the backend is unpublished behind the trusted `web` proxy); leave **off** for any directly-exposed backend — it is a tenant-spoofing hole otherwise (ADR-0004 §7). |

### The JWT signing secret (required)

The instance **refuses to start** without a JWT signing secret of at least 32
bytes. Generate a strong one and provide it via the environment:

```sh
export YAADEGAR_AUTH_JWT_SECRET="$(openssl rand -base64 48)"
```

Keep it stable across restarts (rotating it invalidates every existing session)
and secret. At least one login method must be enabled and configured or the
instance won't start; username+password (`YAADEGAR_AUTH_PASSWORD_ENABLED`,
default on) satisfies this.

### The public link base (emailed links)

Set `YAADEGAR_PUBLIC_LINK_BASE` to the public URL of the giver-facing site so the
links in reservation-confirm and decay keep/release emails point somewhere real,
e.g. `https://example.com`. Without it those emails have no usable link.

### Outgoing email (SMTP)

If `YAADEGAR_SMTP_HOST` is empty, Yaadegar **logs** emails instead of sending them
(the dev default). To actually send:

| Setting | Env var | Notes |
| --- | --- | --- |
| Host | `YAADEGAR_SMTP_HOST` | e.g. `smtp.example.com`. |
| Port | `YAADEGAR_SMTP_PORT` | `587` (STARTTLS) or `465` (implicit TLS). |
| TLS mode | `YAADEGAR_SMTP_TLS_MODE` | `starttls`, `tls`, or `none` (plaintext, loopback only). |
| Username | `YAADEGAR_SMTP_USERNAME` | Relay auth user (e.g. a mailbox address). |
| Password | `YAADEGAR_SMTP_PASSWORD` | **Environment only.** Use an app password where the provider offers one. |
| From | `YAADEGAR_SMTP_FROM` | Envelope/header From, e.g. `no-reply@example.com`. |

### A minimal production environment

Illustrative values — all placeholders:

```sh
# Backend (app)
YAADEGAR_STORAGE_DRIVER=postgres
YAADEGAR_STORAGE_DSN=postgres://yaadegar:<db-password>@db:5432/yaadegar?sslmode=require
YAADEGAR_BASE_DOMAIN=example.com
YAADEGAR_TRUST_FORWARDED_HOST=true
YAADEGAR_AUTH_JWT_SECRET=<32+-byte-random-secret>
YAADEGAR_PUBLIC_LINK_BASE=https://example.com
YAADEGAR_SMTP_HOST=smtp.example.com
YAADEGAR_SMTP_FROM=no-reply@example.com
YAADEGAR_SMTP_PASSWORD=<smtp-app-password>

# Frontend (web)
BACKEND_ORIGIN=http://app:8080          # internal address of the app service
ORIGIN=https://example.com              # the public origin browsers use (CSRF check)
PROTOCOL_HEADER=x-forwarded-proto       # behind a TLS-terminating proxy (ADR-0006 §5)
HOST_HEADER=x-forwarded-host
```

## Bootstrap the first tenant, owner, and admin

Owner self-registration isn't built yet, so you seed the first tenant and owner
from the CLI — the same `yaadegar` binary, run inside the running backend
container (where it inherits the storage configuration from the environment):

```sh
# 1. Create a tenant (addressed as <subdomain>.<base-domain>).
docker compose exec app yaadegar create-tenant --subdomain alice

# 2. Create an owner with a password credential in that tenant.
docker compose exec app yaadegar create-owner \
  --tenant alice --username alice --password '<initial-password>'
```

`create-owner` also accepts optional `--email` and `--name`. The password is
stored only as an argon2id hash.

### Granting the instance-admin capability

Admin is a **capability on an existing owner account** (ADR-0010), not a separate
login — there is no admin username or password. Grant it with the CLI; that owner
then reaches `/admin` with their ordinary owner session:

```sh
docker compose exec app yaadegar grant-admin --tenant alice --username alice
# ...and to remove it later:
docker compose exec app yaadegar grant-admin --tenant alice --username alice --revoke
```

### Password recovery

To reset an owner's (or admin's) password without their old one — the login
lockout-recovery path — use `set-password`. It reads the new password from
`$YAADEGAR_PASSWORD` (or stdin), so the secret never lands in shell history:

```sh
YAADEGAR_PASSWORD='<new-password>' docker compose exec -e YAADEGAR_PASSWORD app \
  yaadegar set-password --tenant alice --username alice
```

> Running the binary directly (not via compose) instead? Every seed/operator
> command takes `--storage-driver` and `--storage-dsn` (or their env vars) to
> reach the same database the server uses.

## Reverse proxy, custom domains, and TLS

For anything public, front the **`web`** service with a TLS-terminating reverse
proxy. The `app` service stays unpublished; only `web` sits behind the proxy.

### TLS termination and forwarded headers

Terminate TLS at the proxy and forward to `web` on its internal port (`3000`),
passing the original scheme and host. The `web` service reads them via
`PROTOCOL_HEADER=x-forwarded-proto` and `HOST_HEADER=x-forwarded-host`, which is
what yields per-tenant `https` origins and lets it set the `Secure` cookie flag
(ADR-0006 §5). Set `ORIGIN` to the public origin the browser uses.

A sketch (nginx-style, placeholders):

```nginx
server {
    listen 443 ssl;
    server_name example.com *.example.com;   # base domain + tenant subdomains

    # ssl_certificate / ssl_certificate_key for a wildcard cert covering *.example.com

    location / {
        proxy_pass         http://web:3000;
        proxy_set_header   Host              $host;
        proxy_set_header   X-Forwarded-Host  $host;
        proxy_set_header   X-Forwarded-Proto $scheme;
    }
}
```

For multi-tenant subdomains you need **wildcard DNS** (`*.example.com`) and a
**wildcard TLS certificate** (`*.example.com`), since each tenant is its own
subdomain. (Caddy or Traefik with an ACME DNS-01 challenge can automate the
wildcard cert.)

### Custom domains

An owner can point their own domain at a list surface. This is **self-service in
the owner's Settings** (no operator action per domain), backed by
`POST /api/v1/domains`:

1. Set the instance's CNAME target once, via `YAADEGAR_DOMAIN_CNAME_TARGET` (e.g.
   `domains.example.com`) — this is the hostname owners point their CNAME at.
2. The owner adds their domain in **Settings → custom domain**; Yaadegar returns a
   DNS challenge.
3. The owner publishes it as a **TXT record at `_yaadegar-verify.<their-hostname>`**
   and points their domain's **CNAME at your `DOMAIN_CNAME_TARGET`**.
4. The owner clicks **Verify**; Yaadegar checks the TXT record and activates the
   domain.

Your reverse proxy/TLS layer must be able to serve a certificate for verified
custom domains (e.g. on-demand ACME issuance keyed to verified hostnames). An
unverified claim only holds its hostname for `YAADEGAR_DOMAIN_CLAIM_TTL` before
another tenant can reclaim it; a verified domain is never reclaimed (ADR-0004).

## Google OAuth setup

Google sign-in is an **add-on** to password login (ADR-0008), configured once at
the instance level and then enabled **per tenant**.

### 1. Create the OAuth client in Google Cloud

1. In the [Google Cloud Console](https://console.cloud.google.com/), select or
   create a project.
2. Configure the **OAuth consent screen** (external, with your app name and
   support email).
3. Go to **APIs & Services → Credentials → Create Credentials → OAuth client ID**,
   type **Web application**.
4. Under **Authorized redirect URIs**, add **exactly one** URI:

   ```
   https://<your-domain>/api/v1/auth/oauth/google/callback
   ```

   `https://<your-domain>` is a **single fixed host** you dedicate to the OAuth
   callback (e.g. `https://example.com`) — it becomes your `redirect-base`. Because
   Google requires each redirect URI
   to be registered exactly and doesn't allow wildcards, Yaadegar uses one fixed
   callback host and then bounces the completed login back to the tenant that
   started it — so you register **one** redirect URI here regardless of how many
   tenant subdomains you run.
5. Copy the generated **client ID** and **client secret**.

### 2. Wire it into the instance config

Provide all three values; the client **secret comes from the environment**. A
partial config (some but not all three) makes the instance refuse to start
(fail-closed); setting none simply leaves Google login off.

```sh
YAADEGAR_OAUTH_GOOGLE_CLIENT_ID=<client-id>
YAADEGAR_OAUTH_GOOGLE_CLIENT_SECRET=<client-secret>     # environment only
YAADEGAR_OAUTH_REDIRECT_BASE=https://example.com        # matches the registered URI's base
```

Restart the instance so it picks up the new configuration.

### 3. Enable Google login per tenant

OAuth is gated per tenant. Turn it on for each tenant that should offer Google
sign-in:

```sh
docker compose exec app yaadegar enable-tenant-oauth --tenant alice
# ...and to turn it off again:
docker compose exec app yaadegar enable-tenant-oauth --tenant alice --disable
```

A tenant's login page then offers "Continue with Google" alongside password login.

## Verify a healthy instance

- **Health probe.** The container healthcheck runs `yaadegar health`, which probes
  `/healthz` and exits non-zero if unhealthy. Check container status with
  `docker compose ps` (the `app` service should be `healthy`).
- **Auth methods.** `GET https://<subdomain>.example.com/api/v1/auth/methods`
  returns `200` and lists the enabled login methods (password, and `google` where
  you enabled it per tenant). This also confirms the public `/api/v1` proxy is
  working.
- **Owner login + settings.** Log in at `https://alice.example.com` with the owner
  you created, and open **Settings** — it should load without error.
- **Giver surface.** Create a list, copy its share link, and open it in a
  logged-out browser: you should see the public giver page and be able to reserve
  an item anonymously.

## Upgrades and backups

- **Upgrades.** Pull a newer image (or rebuild) and restart; the backend applies
  any new migrations automatically on start. Review the release notes for a
  version before upgrading.
- **Backups.** Back up your PostgreSQL database (or the SQLite file) regularly —
  it holds all tenants, lists, reservations, and credentials. Test a restore.

---

For deeper design detail, the [ADRs](adr/) cover custom domains (ADR-0004),
owner authentication (ADR-0005), the frontend + TLS model (ADR-0006), reserver
identity (ADR-0007), Google OAuth (ADR-0008), and the instance-admin model
(ADR-0010).
