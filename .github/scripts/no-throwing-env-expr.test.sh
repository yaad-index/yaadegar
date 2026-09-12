#!/usr/bin/env bash
#
# Tests for no-throwing-env-expr.sh (#375).
#
# 🚨 THE NEGATIVE CONTROL IS THE POINT OF THIS FILE, not a formality. A structural
# check on workflow text is fiddly enough to pass vacuously, and a vacuous always-on
# check is worse than no check: it converts an unexamined area into one that looks
# examined. So the first case below feeds the check the EXACT line that broke
# release-please in #373, and the check must FAIL on it. Every other case is only
# meaningful once that one does.
#
# The pre-fix line is reproduced here verbatim rather than read from git history, so
# the suite works in a shallow CI checkout and cannot quietly stop exercising the
# case it exists for.
set -uo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
script="${here}/no-throwing-env-expr.sh"
failures=0

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

fixture() { # name, then body on stdin
  local f="${tmp}/$1.yml"
  cat > "$f"
  printf '%s' "$f"
}

assert_status() {
  if [ "$1" -eq "$2" ]; then echo "  ok: $3"; else echo "  FAIL: $3 (exit $1, wanted $2)"; failures=$(( failures + 1 )); fi
}
assert_contains() {
  if [[ "$1" == *"$2"* ]]; then echo "  ok: $3"; else echo "  FAIL: $3"; echo "    wanted: $2"; echo "    got: $1"; failures=$(( failures + 1 )); fi
}

echo "🚨 NEGATIVE CONTROL: the exact pre-fix line from #373 must FAIL"
f="$(fixture prefix <<'YML'
jobs:
  release-please:
    steps:
      - name: Request reviewers on the release pull request
        if: steps.release.outputs.prs_created == 'true'
        env:
          PR_NUMBER: ${{ fromJSON(steps.release.outputs.pr).number }}
        run: .github/scripts/request-pr-reviewers.sh
YML
)"
out="$(bash "$script" "$f" 2>&1)"; code=$?
assert_status "$code" 1 "exits 1 on the line that broke the workflow"
assert_contains "$out" "unguarded fromJSON" "names the defect"
assert_contains "$out" "steps.release.outputs.pr && fromJSON(steps.release.outputs.pr)" "shows the guarded form to write instead"
assert_contains "$out" ":7:" "reports the line number"

echo "the post-fix line passes"
f="$(fixture postfix <<'YML'
jobs:
  release-please:
    steps:
      - env:
          PR_NUMBER: ${{ steps.release.outputs.pr && fromJSON(steps.release.outputs.pr).number }}
YML
)"
out="$(bash "$script" "$f" 2>&1)"; code=$?
assert_status "$code" 0 "exits 0"
assert_contains "$out" "no unguarded fromJSON" "says what it checked"

echo "a guard on a DIFFERENT operand does not count"
# ⚠️ The guard has to short-circuit the value actually being parsed. Guarding on a
# neighbouring expression reads as careful and protects nothing.
f="$(fixture wrongoperand <<'YML'
jobs:
  j:
    steps:
      - env:
          X: ${{ steps.other.outputs.flag && fromJSON(steps.release.outputs.pr).number }}
YML
)"
out="$(bash "$script" "$f" 2>&1)"; code=$?
assert_status "$code" 1 "exits 1"
assert_contains "$out" "unguarded fromJSON" "flags it"

echo "spacing around && does not matter"
f="$(fixture spacing <<'YML'
jobs:
  j:
    steps:
      - env:
          A: ${{ steps.s.outputs.v&&fromJSON(steps.s.outputs.v).n }}
          B: ${{ steps.s.outputs.w   &&   fromJSON(steps.s.outputs.w).n }}
YML
)"
out="$(bash "$script" "$f" 2>&1)"; code=$?
assert_status "$code" 0 "exits 0 for both spellings"

echo "two calls on one line: the unguarded one is still reported"
f="$(fixture twocalls <<'YML'
jobs:
  j:
    steps:
      - env:
          A: ${{ steps.s.outputs.v && fromJSON(steps.s.outputs.v).n }}-${{ fromJSON(steps.s.outputs.w).n }}
YML
)"
out="$(bash "$script" "$f" 2>&1)"; code=$?
assert_status "$code" 1 "exits 1"
assert_contains "$out" "fromJSON(steps.s.outputs.w)" "names the unguarded call, not the guarded one"

echo "a comment quoting the broken form is not flagged"
# The workflow comments in this repository quote the broken line deliberately, to
# explain why the guard exists. Flagging prose would teach people to stop writing it.
f="$(fixture comment <<'YML'
jobs:
  j:
    steps:
      # This broke once: PR_NUMBER: ${{ fromJSON(steps.release.outputs.pr).number }}
      - env:
          A: ${{ steps.s.outputs.v && fromJSON(steps.s.outputs.v).n }}
YML
)"
out="$(bash "$script" "$f" 2>&1)"; code=$?
assert_status "$code" 0 "exits 0"

echo "a workflow with no fromJSON at all passes"
f="$(fixture clean <<'YML'
jobs:
  j:
    steps:
      - env:
          A: ${{ github.repository }}
YML
)"
out="$(bash "$script" "$f" 2>&1)"; code=$?
assert_status "$code" 0 "exits 0"

echo "🚨 an empty file list is refused rather than reported as a pass"
# ⚠️ This is the vacuous-pass guard. Pointing the check at nothing must not look
# like a clean bill of health, or the check can be disabled by a glob that stops
# matching — silently, and in exactly the direction nobody would notice.
out="$(cd "$tmp" && bash "$script" 2>&1)"; code=$?
assert_status "$code" 1 "exits 1 when there are no workflow files to scan"
assert_contains "$out" "examines nothing" "says why that is a failure"

echo "the repository's own workflows pass"
# Runs the check exactly as CI does: no arguments, from the repo root.
out="$(cd "${here}/../.." && bash .github/scripts/no-throwing-env-expr.sh 2>&1)"; code=$?
assert_status "$code" 0 "exits 0 on the real .github/workflows"
assert_contains "$out" "no unguarded fromJSON in" "scanned the real files"
if [[ "$out" =~ no\ unguarded\ fromJSON\ in\ ([0-9]+)\ workflow ]]; then
  n="${BASH_REMATCH[1]}"
  if [ "$n" -ge 5 ]; then echo "  ok: scanned ${n} workflow files"; else echo "  FAIL: only scanned ${n} workflow files — the glob is probably wrong"; failures=$(( failures + 1 )); fi
fi

echo
if [ "$failures" -ne 0 ]; then echo "${failures} assertion(s) failed"; exit 1; fi
echo "all assertions passed"
