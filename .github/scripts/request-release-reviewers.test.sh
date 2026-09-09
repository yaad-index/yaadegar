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

# run_case drives the script against a stub `gh` whose read-back answer is
# scripted. STUB_RETURNS is what the second call reports as the requested set;
# STUB_POST_FAILS makes the request itself fail.
run_case() {
  local returns="$1" post_fails="${2:-no}" pr_json="${3:-{\"number\":123\}}"
  local dir; dir="$(mktemp -d)"

  cat > "${dir}/gh" <<'STUB'
#!/usr/bin/env bash
for arg in "$@"; do
  if [ "$arg" = "POST" ]; then
    if [ "${STUB_POST_FAILS}" = "yes" ]; then
      echo "stub: the request failed" >&2
      exit 1
    fi
    # A request that reports nothing and changes nothing looks exactly like one
    # that worked, which is the whole point of the read-back.
    exit 0
  fi
done
printf '%s\n' "${STUB_RETURNS}"
STUB
  chmod +x "${dir}/gh"

  STUB_RETURNS="$returns" \
  STUB_POST_FAILS="$post_fails" \
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

echo "both reviewers present after the request"
out="$(run_case 'first-reviewer second-reviewer')"; code=$?
assert_status "$code" 0 "exits 0"
assert_contains "$out" "has its reviewers requested" "reports what it verified"

echo "🚨 a request that changes NOTHING and reports no error still fails"
out="$(run_case '')"; code=$?
assert_status "$code" 1 "exits 1"
assert_contains "$out" "are not requested on pull request #123" "names the pull request"
assert_contains "$out" "did not take effect" "says the request was the thing that failed, not the reviewers"

echo "a partial request names only the reviewer that is missing"
out="$(run_case 'first-reviewer')"; code=$?
assert_status "$code" 1 "exits 1"
assert_contains "$out" "second-reviewer are not requested" "names the missing one"
assert_contains "$out" "requested set is now 'first-reviewer'" "and shows what did land"

echo "a failing request is not swallowed"
out="$(run_case 'first-reviewer second-reviewer' yes)"; code=$?
assert_status "$code" 1 "exits 1"

echo "a created pull request with no usable number is refused rather than guessed"
out="$(run_case 'first-reviewer second-reviewer' no '{"title":"chore: release"}')"; code=$?
assert_status "$code" 1 "exits 1"
assert_contains "$out" "carries no usable number" "says which input was unusable"
assert_contains "$out" "refusing to guess" "and does not act on a guess"

echo "a number that is not a number is refused too, not only an absent one"
# ⚠️ Added after a mutation survived: removing the non-numeric arm broke nothing,
# because the only case reaching that check supplied NO number and was caught by
# the empty arm. An arm with no case is untested rather than safe.
out="$(run_case 'first-reviewer second-reviewer' no '{"number":"not-a-number"}')"; code=$?
assert_status "$code" 1 "exits 1"
assert_contains "$out" "carries no usable number" "says which input was unusable"

echo
if [ "$failures" -ne 0 ]; then echo "${failures} assertion(s) failed"; exit 1; fi
echo "all assertions passed"
