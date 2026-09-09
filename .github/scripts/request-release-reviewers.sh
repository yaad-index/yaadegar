#!/usr/bin/env bash
#
# Request reviewers on a pull request and verify they are actually there (#343).
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
#   PR_JSON    the release-please `pr` output; the number is read from it
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
# remains unexercised is the release-please `pr` output's shape, which only exists
# on a run that actually opens a release pull request, and whether the token in use
# may request reviewers. Both first execute at the next release.
set -euo pipefail

: "${REPO:?REPO must name the repository}"
: "${PR_JSON:?PR_JSON must carry the created pull request}"
: "${REVIEWERS:?REVIEWERS must list the logins to request}"

pr="$(printf '%s' "$PR_JSON" | jq -r '.number // empty')"
case "$pr" in
  '' | *[!0-9]*)
    echo "a pull request was reported as created but carries no usable number (#343): got '${pr}' from the release output — refusing to guess which pull request to act on"
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
# already approved can reset that approval, and a release pull request whose
# approvals evaporate whenever trunk moves reads as reviewers being slow rather
# than as an automation undoing their work.
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
requested="$(gh api "repos/${REPO}/pulls/${pr}/requested_reviewers" | jq -r '[.users[].login] | join(" ")')"
reviewed="$(gh api "repos/${REPO}/pulls/${pr}/reviews" | jq -r '[.[].user.login] | unique | join(" ")')"
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
