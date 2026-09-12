#!/usr/bin/env bash
#
# Request reviewers on a pull request and verify they are actually there (#343).
#
# 🚨 THE SCOPE IS A CLASS, NOT THE RELEASE PULL REQUEST. Two workflows open pull
# requests under an identity that cannot approve them — release-please.yml cuts the
# release PR, docs-compose.yml opens the docs pin bump that follows every release —
# and both used to arrive with nobody requested. Special-casing this to the release
# PR is what left the pair half-solved, so this takes a pull request NUMBER and
# nothing about where it came from. The next automation to open one wires in the
# same way instead of inheriting the same silence.
#
# Lives in a file rather than inside a `run:` block so it can be run against a
# stub `gh` and asserted — the lesson of #359, where a poll loop embedded in
# workflow YAML could not be invoked by anything and its regressions would have
# been invisible. Writing a new untested run-block here would reintroduce exactly
# what that change removed.
#
# Everything is environment:
#
#   REPO       owner/name
#   PR_NUMBER  the pull request to act on
#   REVIEWERS  space-separated logins to request
#   GH_TOKEN   consumed by `gh` itself
#
# `gh` is invoked by name so a test can put a stub earlier on PATH. There is no
# indirection for it: production runs the same bytes the test runs.
#
# ⚠️ WHAT THE TESTS DO NOT COVER, named precisely so a green suite is not read as
# more than it is. The stub answers the two GET endpoints with API-SHAPED payloads
# and the jq below runs against them, so the parse IS exercised — and both shapes
# were confirmed against the live API rather than taken from documentation. What
# remains unexercised:
#
#   - whether the token in use may request reviewers, for either caller;
#   - `--paginate` actually following a next link, which is `gh`'s behaviour and
#     not this script's. The stub answers in one page, so the flag's effect cannot
#     be asserted here. It was confirmed directly against the live API instead:
#     the reviews endpoint at `per_page=2` returns 2 of 6 reviews without it and
#     all 6 with it, merged into a single array.
#
# Both first execute on a real run.
set -euo pipefail

# ⚠️ `?` rather than `:?` on the two that have their OWN guard below, and the
# difference is not cosmetic. `:?` fires on set-but-empty, which is the exact case
# a caller produces when it derives a value from an output that came back blank —
# and it fires FIRST, so the explicit checks below never run and their messages
# never appear. On main both of those guards were unreachable that way: the empty
# arm for the number, and the empty-list arm for the reviewers. A guard an earlier
# line makes unreachable is dead code that reads as protection.
#
# So: these must be SET (a caller that forgot one is a wiring bug, caught here),
# but an empty VALUE is passed through to the check written for it.
: "${REPO:?REPO must name the repository}"
: "${PR_NUMBER?PR_NUMBER must be set to the pull request to act on}"
: "${REVIEWERS?REVIEWERS must be set to the logins to request}"

# ⚠️ Guard the number rather than trusting the caller, and keep BOTH arms. Callers
# derive it from an API response or a workflow output, so an absent value and a
# non-numeric one are different failures and each reaches here on its own path. An
# earlier version checked only for empty, and dropping the non-numeric arm broke no
# test because nothing exercised it — an arm with no case is untested, not safe.
pr="$PR_NUMBER"
case "$pr" in
  '' | *[!0-9]*)
    echo "a pull request was reported as opened but carries no usable number (#343): got '${pr}' — refusing to guess which pull request to act on"
    exit 1
    ;;
esac

read -r -a wanted <<< "$REVIEWERS"
if [ "${#wanted[@]}" -eq 0 ]; then
  echo "REVIEWERS is empty (#343): a step that requests nobody and reports success is the silence this exists to end"
  exit 1
fi

# 🚨 Read the current state BEFORE asking for anything, and request only who is
# missing from it. This is what makes the step idempotent BY CONSTRUCTION rather
# than by a claim about when the caller runs it: re-requesting someone who has
# already approved can reset that approval, and a pull request whose approvals
# evaporate whenever trunk moves reads as reviewers being slow rather than as an
# automation undoing their work.
#
# ⚠️ This matters unequally across the two callers, and the weaker case is the one
# to write for. release-please rewrites the SAME pull request on every push to the
# default branch, so its step runs repeatedly against an already-open PR with live
# approvals on it. docs-compose opens a fresh branch per release, so its PR is
# created once and a plain request-on-create would have been enough there. Reading
# first is correct for both, which is why both callers run this unchanged rather
# than one carrying a relaxed variant that is fine until it is reused.
#
# ⚠️ "Covered" is requested OR already reviewed, and the second half is not
# optional. **GitHub removes a reviewer from `requested_reviewers` once they
# submit a review** — observed live: a pull request with one review and one
# pending request returns only the pending one. So absent-from-requested means
# either "never asked" or "already answered", and treating those alike would
# re-request exactly the person whose approval is at stake. Two states, one
# rendering, and the reviews list is what separates them.
#
# The jq runs here rather than inside `gh --jq` so a stub can emit an API-shaped
# payload and the parse itself is exercised. Both shapes below are confirmed
# against the live API rather than taken from documentation.
#
# ⚠️ No `--paginate` on this one, and that is a decision rather than an omission.
# It returns an OBJECT (`{"users":[],"teams":[]}`) rather than a page of a list,
# and GitHub caps a pull request at 15 requested reviewers, which is inside the
# first page by construction. Adding the flag here for symmetry with the reviews
# call below would be an argument from layout against an endpoint whose shape does
# not paginate.
requested="$(gh api "repos/${REPO}/pulls/${pr}/requested_reviewers" | jq -r '[.users[].login] | join(" ")')"
#
# ⚠️ DISMISSED reviews do not count as covered, and this is a decision rather than
# a consequence of the union. This repository dismisses stale reviews on a diff
# change, and release-please rewrites the changelog and manifest on every push to
# main — so a release pull request's approvals are dismissed repeatedly while it is
# open. Counting a dismissed review as an answer would leave that reviewer neither
# requested nor approving, with nothing pending against their name: the same
# silence this step exists to end, arriving one step later. Excluding them means
# they are re-requested and re-notified, and there is no approval left to reset
# because it has already been dismissed.
#
# 🚨 `--paginate` IS required here, and the consequence of losing it is silent
# rather than loud. This endpoint returns a LIST, 30 per page by default, oldest
# first — so once a pull request carries more than a page of reviews, the newest
# ones are the ones that fall off. A reviewer whose approval is on a later page
# reads as never having reviewed, which re-requests them and resets the very
# approval the union above exists to protect: a wrong answer that looks like a
# normal one. Dismissals accumulate here rather than replacing each other, and
# this repository dismisses on every release-please rewrite, so a long-lived
# release PR is exactly where the count climbs. `gh api --paginate` merges the
# pages into a single array, confirmed live, so the jq below is unchanged by it.
reviewed="$(gh api --paginate "repos/${REPO}/pulls/${pr}/reviews" | jq -r '[.[] | select(.state != "DISMISSED") | .user.login] | unique | join(" ")')"
covered=" ${requested} ${reviewed} "

ask=()
for who in "${wanted[@]}"; do
  case "$covered" in
    *" ${who} "*) ;;
    *) ask+=("${who}") ;;
  esac
done

if [ "${#ask[@]}" -eq 0 ]; then
  echo "pull request #${pr} already has every reviewer requested or reviewing: requested '${requested}', reviewed '${reviewed}'"
  exit 0
fi

# ⚠️ The array form is required. `gh pr edit --add-reviewer` REPLACES the reviewer
# set rather than appending to it, so it cannot reliably add two.
args=()
for who in "${ask[@]}"; do
  args+=(-f "reviewers[]=${who}")
done
gh api "repos/${REPO}/pulls/${pr}/requested_reviewers" -X POST "${args[@]}" > /dev/null

# 🚨 Read back rather than trusting the call. A reviewer request can return
# without error and leave the set untouched, so the check is the state of the pull
# request and not the exit code of the request. Without this the fix has exactly
# the shape of the bug it removes: an automation whose failure is invisible.
#
# This is the part that must not be relaxed for either caller. The docs pin PR is
# opened once and never rewritten, which makes its idempotency story simpler, but
# it gains nothing from skipping the verification — a request that reports no error
# and changes nothing looks identical on both.
requested="$(gh api "repos/${REPO}/pulls/${pr}/requested_reviewers" | jq -r '[.users[].login] | join(" ")')"
covered=" ${requested} ${reviewed} "

missing=()
for who in "${wanted[@]}"; do
  case "$covered" in
    *" ${who} "*) ;;
    *) missing+=("${who}") ;;
  esac
done

if [ "${#missing[@]}" -ne 0 ]; then
  echo "reviewers ${missing[*]} are not requested on pull request #${pr} after asking for them (#343): the request reported no error and did not take effect — the requested set is now '${requested}'"
  exit 1
fi

echo "pull request #${pr} has its reviewers requested: asked for '${ask[*]}', set is now '${requested}'"
