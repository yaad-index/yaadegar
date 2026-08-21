# ADR-0014: Publishing the web image (two images, released as a matched pair)

**Status:** Proposed

## Context

`docker-publish.yml` builds and pushes exactly one image, `ghcr.io/yaad-index/yaadegar`, from the
repository root context. That Dockerfile is multi-stage onto a digest-pinned
`distroless/static-debian12:nonroot` base and copies in a single artifact, the Go binary, which
embeds its own migrations. Nothing else ships in it.

The SvelteKit frontend is a **separate service** with its own `web/Dockerfile` — adapter-node on a
slim Node base, listening on 3000, reading `BACKEND_ORIGIN` and `ORIGIN`. **No workflow publishes
it.**

The consequence is only visible when you ask what a self-hoster ends up with. A `docker-compose.yml`
written against published images alone starts a healthy container serving the API on 8080 **and no
website**. It starts, it reports healthy, and it is not what the operator wanted. Nothing signals
the gap, which makes it a worse failure than an obvious one.

The repository's own `docker-compose.yml` hides this, and not by accident: it uses `build:` for both
`app` and `web`, so the compose that works is the one that never references a published image. The
only path to a working instance today is a source checkout that builds both services.

Two properties of the runtimes are load-bearing for the decision below and are not incidental:

- The API runs **distroless**: no shell, no package manager, non-root, bases pinned by digest.
- The frontend **requires a Node runtime at run time**, because public share pages are
  server-rendered (ADR-0006 §1). A static adapter is not available to it.

This blocks describing an images-only quick start honestly (#258, blocking #236).

## Decision

### 1. Two images, not one combined image

Publish a second image, **`ghcr.io/yaad-index/yaadegar-web`**, built from the `web/` context,
alongside the existing API image.

A single combined image was considered and rejected. Its one genuine advantage is that version skew
between frontend and backend becomes impossible by construction. The costs are concrete and fall on
properties the project already chose deliberately:

- The frontend needs Node at run time, so one image means **two processes in one container** and
  therefore a supervisor — a new runtime dependency and a new failure mode, in a project whose API
  image currently contains one binary and nothing else.
- The API would **lose its distroless base**: the Go binary would have to move into the Node image,
  giving up the no-shell, no-package-manager, digest-pinned runtime.
- **Seeding becomes ambiguous.** Tenants, owners and the admin capability are established with
  `docker compose exec app yaadegar <subcommand>`, which reads clearly while `app` is one service
  and one entrypoint.

Skew is the only thing the combined image buys, and §2 and §3 buy it far more cheaply.

There is also a deployment a combined image forecloses. Yaadegar is API-first (ADR-0001), so an
operator may legitimately want to run **the API and no website** — fronting it with their own
client, or serving only programmatic consumers. That is a valid instance, and it is the case #258 is
*not* about: the bug is an operator who wanted a site and silently got none, not one who wanted no
site. Two images keep both deployments expressible; a combined image would impose a Node frontend on
someone who never renders a page from it.

### 2. The two images are released as a matched pair

- Both are built and pushed **in the same workflow run**, from the same `docker/metadata-action`
  step, so they carry an identical tag set. `:0.13.0` denotes the same release on both.
- **The web image gets a version stamp.** It has none today. `web/Dockerfile` takes a `VERSION`
  build-arg, baked in as an environment value the server reports, mirroring the `-X main.version`
  link-time stamp on the API.
- **The existing pre-publish guards extend to it.** The `#190`/`#193` assertions — resolved version
  is semver-shaped, and the built image reports that version back before anything is pushed — cover
  the API alone. The web image gets the same single-arch build-load-read check.
- **Neither image is pushed until both verify.** A half-published release that moves `:latest` on
  one image and not the other is precisely the mismatched pair this ADR exists to prevent.

### 3. Skew is detectable at run time, not merely discouraged

Lockstep tagging is a convention that nothing currently enforces: an operator is free to pin
`:latest` on one image and a version on the other, and today nothing would say so. There is a
`/healthz` endpoint but **no version surface at all**.

- **A new unauthenticated `GET /version` endpoint** reports the API's version, from the same stamp
  the `version` subcommand reads.
- **It is deliberately a new route rather than a field on `/healthz`.** `/healthz` returns
  `text/plain` `ok`; giving it a version field means turning it into JSON, which is a **content-type
  break** for any operator health check that reads the body, not an additive change. Breaking
  self-hosters' monitoring to fix a self-hosting bug is the wrong trade, and a liveness probe that
  also carries build metadata is doing two jobs.
- The **web service reads it once at startup** and, on mismatch with its own stamp, logs an explicit
  error naming both versions.
- **It logs and continues; it does not refuse to start.** Refusing would convert a cosmetic mismatch
  during a staged rollout into an outage. The complaint this ADR answers is silence, not tolerance.
- **A startup log line alone would not be enough**, because it is written once, to a stream nobody
  may be reading. `/version` is the part that makes skew *externally* observable: an operator's
  existing uptime check can poll both services and compare, with no access to container logs. The
  log line is the loud signal at the moment of the mistake; the endpoint is what can be monitored
  continuously afterwards.

### 4. The documented compose is executed before it is published

A quick-start compose is copy-pasted rather than read, so it has to be **run**, not reviewed.

- A self-hosting compose referencing both images **by version tag** ships in the docs, and CI stands
  it up end to end and asserts the site is served — not only that containers start, which is exactly
  the signal that fails to distinguish this bug from a working instance.
- **This job is the most expensive and least deterministic thing in the workflow**, and that is
  accepted with its eyes open: it pulls two published images, waits on Postgres and two health
  gates, and asserts on a rendered response, so it is both slower than the existing build guards and
  the likeliest to flake. It runs on release and on changes to the compose or the Dockerfiles rather
  than on every push, and a flake here must be fixed rather than retried away — a quick start that
  passes only on a re-run is not one an operator can trust.
- **The backend port stays unpublished.** `YAADEGAR_TRUST_FORWARDED_HOST` is safe only because the
  backend is not externally reachable and the web service is its sole trusted proxy (ADR-0004 §7).
  A compose that publishes 8080 for convenience silently breaks that trust model while appearing to
  work, so this constraint is documented in the file itself rather than left implicit.
- The existing root `docker-compose.yml` is **unchanged** and remains the contributor path, building
  both services from source.

## Consequences

- An images-only quick start becomes honest, and #236 can describe one without instructing a reader
  to clone and build.
- **Cost:** a second multi-arch image to build and push on every release and every push to main,
  lengthening the publish job and adding registry storage.
- **A new failure mode is created, not eliminated:** two independently-pinnable images can skew.
  §2 makes a matched pair the default and §3 makes a mismatched one say so, but an operator who
  deliberately pins them apart is not prevented from doing it.
- The web image is Node-based and therefore substantially larger than the distroless API image. That
  is inherent to server-rendered pages and is accepted rather than worked around.
- **`/healthz` is left alone**, so no existing health check changes behaviour. The one in-repo
  consumer is the `health` CLI subcommand used as the container healthcheck, and it reads only the
  status code — but operators' own probes are not in this repository and may well read the body,
  which is why the version surface is a separate route.
- **`/version` is a new unauthenticated surface that discloses the running build**, which narrows an
  attacker's search for applicable vulnerabilities. Accepted deliberately: the endpoint exists to be
  polled by monitoring that holds no credentials, so gating it behind auth would defeat its purpose,
  and the project is open-source and self-hosted, where the running version is largely inferable
  anyway. It carries the version and nothing else.
- The generated TypeScript client is committed and drift-guarded, so the web image build needs no
  access to the OpenAPI spec and no ordering against the API build.
- **Any future non-web client inherits `/version` as its compatibility check.** The bundled frontend
  is redeployed in lockstep with the instance, so its skew window is a rollout; a client that ships
  independently of the instance it talks to has no such window, and skew becomes a standing
  condition rather than a transient one. The endpoint is specified here for the web service's
  benefit, but it is the surface such a client would need, which is a reason to keep it a plain,
  unauthenticated route rather than something the frontend alone can reach.
