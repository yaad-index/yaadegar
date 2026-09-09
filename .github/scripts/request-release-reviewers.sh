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

# ⚠️ The array form is required. `gh pr edit --add-reviewer` REPLACES the reviewer
# set rather than appending to it, so it cannot reliably add two.
args=()
for who in "${wanted[@]}"; do
  args+=(-f "reviewers[]=${who}")
done
gh api "repos/${REPO}/pulls/${pr}/requested_reviewers" -X POST "${args[@]}" > /dev/null

# 🚨 Read the reviewers back rather than trusting the call. A reviewer request can
# return a malformed response and leave the set untouched while the command still
# looks like it ran — so without this the fix has exactly the shape of the bug it
# removes: an automation whose failure is invisible. The check is the state of the
# pull request, not the exit code of the request.
requested="$(gh api "repos/${REPO}/pulls/${pr}/requested_reviewers" --jq '[.users[].login] | join(" ")')"

missing=()
for who in "${wanted[@]}"; do
  case " ${requested} " in
    *" ${who} "*) ;;
    *) missing+=("${who}") ;;
  esac
done

if [ "${#missing[@]}" -ne 0 ]; then
  echo "reviewers ${missing[*]} are not requested on pull request #${pr} after asking for them (#343): the request reported no error and did not take effect — the requested set is now '${requested}'"
  exit 1
fi

echo "pull request #${pr} has its reviewers requested: ${requested}"
