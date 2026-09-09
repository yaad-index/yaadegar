#!/usr/bin/env bash
#
# Tests for request-release-reviewers.sh (#343).
#
# 🚨 The assertion that matters is the READ-BACK, not the request. The bug this
# script exists to remove is an automation whose failure is invisible, and the
# way this script could reproduce it is a reviewer request that returns without
# error and leaves the set untouched. So the stub is driven to do exactly that,
# and the script must exit non-zero on it.
#
# A run where the request works and the read-back agrees is what a script with no
# read-back at all also produces, so that case alone would prove nothing.
set -uo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
script="${here}/request-release-reviewers.sh"
failures=0

# run_case drives the script against a stub `gh` that answers the two GET
# endpoints with API-SHAPED payloads, so the script's own jq runs against
# something the real API could have returned rather than against a pre-parsed
# string. Both shapes were confirmed against the live API before being encoded
# here.
#
# STUB_REQUESTED / STUB_REVIEWED are the logins each GET reports; STUB_AFTER is
# what the requested endpoint reports once a POST has happened, which is how a
# request that silently changes nothing is expressed.
run_case() {
  local requested="$1" reviewed="$2" after="$3" post_fails="${4:-no}" pr_json="${5:-{\"number\":123\}}"
  local dir; dir="$(mktemp -d)"

  cat > "${dir}/gh" <<'STUB'
#!/usr/bin/env bash
posted="${STUB_STATE}/posted"

to_users() {
  printf '{"users":['
  local first=1
  for who in $1; do
    [ $first -eq 1 ] || printf ','
    printf '{"login":"%s","id":1}' "$who"
    first=0
  done
  printf '],"teams":[]}\n'
}

to_reviews() {
  printf '['
  local first=1
  for who in $1; do
    [ $first -eq 1 ] || printf ','
    printf '{"user":{"login":"%s"},"state":"APPROVED"}' "$who"
    first=0
  done
  printf ']\n'
}

for arg in "$@"; do
  if [ "$arg" = "POST" ]; then
    if [ "${STUB_POST_FAILS}" = "yes" ]; then
      echo "stub: the request failed" >&2
      exit 1
    fi
    : > "$posted"
    exit 0
  fi
done

case "$1$2$3" in
  *reviews*) to_reviews "${STUB_REVIEWED}" ;;
  *) if [ -f "$posted" ]; then to_users "${STUB_AFTER}"; else to_users "${STUB_REQUESTED}"; fi ;;
esac
STUB
  chmod +x "${dir}/gh"

  STUB_REQUESTED="$requested" \
  STUB_REVIEWED="$reviewed" \
  STUB_AFTER="$after" \
  STUB_POST_FAILS="$post_fails" \
  STUB_STATE="$dir" \
  PATH="${dir}:${PATH}" \
  REPO=owner/repo \
  PR_JSON="$pr_json" \
  REVIEWERS="first-reviewer second-reviewer" \
  GH_TOKEN=stub \
    bash "$script"
  local code=$?
  rm -rf "$dir"
  return $code
}

assert_status() {
  if [ "$1" -eq "$2" ]; then echo "  ok: $3"; else echo "  FAIL: $3 (exit $1, wanted $2)"; failures=$(( failures + 1 )); fi
}
assert_contains() {
  if [[ "$1" == *"$2"* ]]; then echo "  ok: $3"; else echo "  FAIL: $3"; echo "    wanted: $2"; echo "    got: $1"; failures=$(( failures + 1 )); fi
}

echo "nobody requested yet: both are asked for, and the result is verified"
out="$(run_case '' '' 'first-reviewer second-reviewer')"; code=$?
assert_status "$code" 0 "exits 0"
assert_contains "$out" "asked for 'first-reviewer second-reviewer'" "asked for both"

echo "🚨 a request that changes NOTHING and reports no error still fails"
out="$(run_case '' '' '')"; code=$?
assert_status "$code" 1 "exits 1"
assert_contains "$out" "are not requested on pull request #123" "names the pull request"
assert_contains "$out" "did not take effect" "says the request failed, not the reviewers"

echo "a partial result names only the reviewer that is missing"
out="$(run_case '' '' 'first-reviewer')"; code=$?
assert_status "$code" 1 "exits 1"
assert_contains "$out" "second-reviewer are not requested" "names the missing one"

echo "already requested: nothing is asked for again"
out="$(run_case 'first-reviewer second-reviewer' '' 'first-reviewer second-reviewer')"; code=$?
assert_status "$code" 0 "exits 0"
assert_contains "$out" "already has every reviewer" "makes no request at all"

echo "🚨 a reviewer who has ALREADY REVIEWED is not re-requested"
# GitHub removes a reviewer from requested_reviewers once they submit a review —
# confirmed live. So absent-from-requested means either "never asked" or "already
# answered", and re-requesting the second can reset the very approval this step
# exists to protect. Only the reviews list separates them.
out="$(run_case '' 'first-reviewer second-reviewer' '')"; code=$?
assert_status "$code" 0 "exits 0"
assert_contains "$out" "already has every reviewer" "asks for nobody"
assert_contains "$out" "reviewed 'first-reviewer second-reviewer'" "and says why"

echo "one reviewed, one never asked: only the missing one is requested"
out="$(run_case '' 'first-reviewer' 'second-reviewer')"; code=$?
assert_status "$code" 0 "exits 0"
assert_contains "$out" "asked for 'second-reviewer'" "asks only for the one with no answer"

echo "a failing request is not swallowed"
out="$(run_case '' '' 'first-reviewer second-reviewer' yes)"; code=$?
assert_status "$code" 1 "exits 1"

echo "a created pull request with no usable number is refused rather than guessed"
out="$(run_case '' '' '' no '{"title":"chore: release"}')"; code=$?
assert_status "$code" 1 "exits 1"
assert_contains "$out" "carries no usable number" "says which input was unusable"

echo "a number that is not a number is refused too, not only an absent one"
# ⚠️ Added after a mutation survived: removing the non-numeric arm broke nothing,
# because the only case reaching that check supplied NO number and was caught by
# the empty arm. An arm with no case is untested rather than safe.
out="$(run_case '' '' '' no '{"number":"not-a-number"}')"; code=$?
assert_status "$code" 1 "exits 1"
assert_contains "$out" "carries no usable number" "says which input was unusable"

echo
if [ "$failures" -ne 0 ]; then echo "${failures} assertion(s) failed"; exit 1; fi
echo "all assertions passed"
