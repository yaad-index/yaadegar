#!/usr/bin/env bash
#
# Tests for require-checkout-before-repo-script.sh (#378).
#
# 🚨 THE NEGATIVE CONTROL IS THE POINT OF THIS FILE, not a formality. The defect this
# check exists for is already fixed, so running the check against the current tree
# proves only that the tree is clean — it cannot distinguish a working check from one
# that reports success unconditionally. **A check that has never been observed to fail
# is indistinguishable from one that cannot.**
#
# So the first case below reproduces the EXACT arrangement that shipped in #368 and
# was fixed in #377 — a release job that runs a script from the working tree with no
# checkout anywhere in it — and the check must FAIL on it. Every other case here is
# only meaningful once that one does.
#
# The pre-fix arrangement is reproduced verbatim rather than read from git history, so
# the suite works in a shallow CI checkout and cannot quietly stop exercising the case
# it exists for.
set -uo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
script="${here}/require-checkout-before-repo-script.sh"
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

echo "🚨 NEGATIVE CONTROL: the pre-fix release job from #368 must FAIL"
f="$(fixture prefix <<'YML'
name: release-please
on:
  push:
    branches: [main]
jobs:
  release-please:
    runs-on: ubuntu-latest
    steps:
      - name: Run release-please
        id: release
        uses: googleapis/release-please-action@45996ed1f6d02564a971a2fa1b5860e934307cf7 # v5.0.0
        with:
          config-file: release-please-config.json

      - name: Request reviewers on the release pull request
        if: steps.release.outputs.prs_created == 'true'
        env:
          REVIEWERS: one-login two-login
        run: .github/scripts/request-pr-reviewers.sh
YML
)"
out="$(bash "$script" "$f" 2>&1)"; st=$?
assert_status "$st" 1 "exits 1 on the arrangement that actually shipped"
assert_contains "$out" "never checks the repository out" "says what is wrong"
assert_contains "$out" "release-please" "names the job it found it in"

echo
echo "the same job WITH a checkout in front of it passes"
f="$(fixture fixed <<'YML'
jobs:
  release-please:
    runs-on: ubuntu-latest
    steps:
      - name: Run release-please
        id: release
        uses: googleapis/release-please-action@45996ed1f6d02564a971a2fa1b5860e934307cf7 # v5.0.0

      - name: Checkout
        uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1

      - name: Request reviewers on the release pull request
        run: .github/scripts/request-pr-reviewers.sh
YML
)"
out="$(bash "$script" "$f" 2>&1)"; st=$?
assert_status "$st" 0 "exits 0"

echo
echo "⚠️ a checkout BELOW the invocation is still a finding — presence is not enough"
f="$(fixture ordering <<'YML'
jobs:
  late:
    runs-on: ubuntu-latest
    steps:
      - name: Request reviewers
        run: .github/scripts/request-pr-reviewers.sh

      - name: Checkout
        uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
YML
)"
out="$(bash "$script" "$f" 2>&1)"; st=$?
assert_status "$st" 1 "exits 1"
assert_contains "$out" "before its checkout" "distinguishes ordering from absence"

echo
echo "🚨 one job checking out does not cover a SIBLING job that does not — the case a"
echo "   whole-file check would pass, and the reason this is scoped per job"
f="$(fixture siblings <<'YML'
jobs:
  has-checkout:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
      - name: Fine
        run: .github/scripts/require-suite-run.sh

  no-checkout:
    runs-on: ubuntu-latest
    steps:
      - name: Broken
        run: .github/scripts/request-pr-reviewers.sh
YML
)"
out="$(bash "$script" "$f" 2>&1)"; st=$?
assert_status "$st" 1 "exits 1"
assert_contains "$out" "no-checkout" "names the job that lacks it"
if [[ "$out" != *"'has-checkout'"* ]]; then echo "  ok: does not flag the job that has one"; else echo "  FAIL: flagged the compliant sibling"; failures=$(( failures + 1 )); fi

echo
echo "🚨 MENTION IS NOT USE: a comment naming the path is not an invocation"
echo "   (the workflow comments explaining this defect quote the path that trips it)"
f="$(fixture mention <<'YML'
jobs:
  documented:
    runs-on: ubuntu-latest
    steps:
      # This job deliberately does not run .github/scripts/request-pr-reviewers.sh,
      # and a check that cannot tell prose from a command would flag this line.
      - name: Unrelated
        run: echo hello
YML
)"
out="$(bash "$script" "$f" 2>&1)"; st=$?
assert_status "$st" 0 "exits 0 on prose that quotes the path"

echo
echo "the path on a later line of a multi-line run block is still an invocation"
f="$(fixture block <<'YML'
jobs:
  inline:
    runs-on: ubuntu-latest
    steps:
      - name: Several things
        run: |
          set -euo pipefail
          echo starting
          PR_NUMBER=7 .github/scripts/request-pr-reviewers.sh
YML
)"
out="$(bash "$script" "$f" 2>&1)"; st=$?
assert_status "$st" 1 "exits 1"

echo
echo "a job that invokes nothing needs no checkout"
f="$(fixture innocent <<'YML'
jobs:
  plain:
    runs-on: ubuntu-latest
    steps:
      - name: Say hello
        run: echo hello
YML
)"
out="$(bash "$script" "$f" 2>&1)"; st=$?
assert_status "$st" 0 "exits 0"

echo
echo "a file whose layout this cannot read is an ERROR, not a pass"
f="$(fixture unreadable <<'YML'
name: nothing here
on: push
YML
)"
out="$(bash "$script" "$f" 2>&1)"; st=$?
assert_status "$st" 1 "exits 1"
assert_contains "$out" "has NOT been checked" "says it examined nothing rather than reporting success"

echo
echo "an empty file list is refused rather than reported as a clean scan"
out="$(cd "$tmp" && bash "$script" 2>&1)"; st=$?
assert_status "$st" 1 "exits 1"
assert_contains "$out" "examines nothing" "says why"

echo
if [ "$failures" -ne 0 ]; then
  echo "${failures} assertion(s) failed"
  exit 1
fi
echo "all assertions passed"
