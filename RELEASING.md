# Releasing

Yaadegar uses [release-please](https://github.com/googleapis/release-please) to
automate versioning and releases from [Conventional
Commits](https://www.conventionalcommits.org/). Versions are git tags
(`vMAJOR.MINOR.PATCH`). The **published image** stamps the release version into
the binary at link time (`#190`) — the version derived from the release tag
(e.g. `0.4.0`, from the `yaadegar-v0.4.0` tag), not the raw tag string — so a
running instance reports its build via the `version` subcommand and the startup
log. A local `make build`, and any plain `docker build` or `docker compose
build` without the `VERSION` build-arg, are not stamped and report `dev`. That
`dev` is the Go source default (`var version = "dev"`), which the Dockerfile's
`ARG VERSION=dev` mirrors so the Docker paths behave the same — an unstamped
local build is honest about being one.

## How it works

1. Merge PRs to `main` using conventional-commit titles (`feat:`, `fix:`,
   `docs:`, `chore:`, …). `feat:` bumps the minor version, `fix:` the patch;
   `feat!:` / a `BREAKING CHANGE:` footer bumps the major.
2. The `release-please` workflow keeps an open **release PR** that accumulates
   the changelog and the next version. It updates itself as more commits land.
3. When you want to cut the release, get the release PR reviewed and merged
   (same gate as any PR: CI green + 2 approvals). Merging it creates the tag,
   the GitHub Release, and updates `CHANGELOG.md`.

The first release starts pre-1.0 from `0.0.0`.

## After the merge: what publishes, and how long it should take

Merging the release PR fires `docker-publish`, which pushes the API and web
images as a matched pair. That workflow has **two** jobs:

1. `require-suite` waits for the `ci.yml` run belonging to the commit being
   published and refuses to continue unless it passed (#356). It exists so an
   image can never be built from a commit no test run has covered.
2. `publish` builds and pushes.

⚠️ **The run's total duration is no longer the publish duration, and old figures
are not comparable to new ones.** `docker-publish` used to have a single job, so
the two numbers were the same and got used interchangeably. The gate's wait —
however long the suite takes on that commit — is now part of the run and not part
of the publish.

**So compare the `publish` JOB against historical numbers, not the run.** A run
several minutes longer than the figures you remember is expected and is not
evidence of a slow build. This has already caused one wrong call: a publish
flagged as overrunning measured 13m22s at the job, against a 12m41s–14m13s range
for the three before it — entirely normal.

🔑 The general form, which outlives this instance: **a number does not have to
move to become wrong — what it was a proxy for can move out from under it.**
Nothing in the old durations marks them as no longer comparable, and the person
most likely to miss that is whoever added the job, because to them the run is
still "the publish".

## First release: the one-time manual step

The release PR is opened by the built-in `GITHUB_TOKEN`, and GitHub
deliberately does **not** run workflows for events triggered by that token. So
the required `check` status does not run on the release PR on its own, and
branch protection won't let it merge until it does.

Until the optional token below is configured, do this once per release PR:

- **Close the release PR, then reopen it.** Reopening it as a real user fires
  the `pull_request` event, so `check` runs. Then approve (2-of-2) and merge as
  usual.

## Optional: remove the manual step

Add a repository secret named `RELEASE_PLEASE_TOKEN` — a fine-grained personal
access token or a GitHub App token with `contents: write` and
`pull_requests: write`. The workflow picks it up automatically (no workflow
edit): the release PR is then authored by that identity, `check` runs normally,
and cutting a release becomes a plain CI-green + approvals merge with no
close/reopen.
