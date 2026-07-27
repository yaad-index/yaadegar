# Releasing

Yaadegar uses [release-please](https://github.com/googleapis/release-please) to
automate versioning and releases from [Conventional
Commits](https://www.conventionalcommits.org/). Versions are git tags
(`vMAJOR.MINOR.PATCH`); the build stamps the tag into the binary at link time.

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
