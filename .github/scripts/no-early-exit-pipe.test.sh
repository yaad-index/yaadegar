#!/usr/bin/env bash
#
# Tests for no-early-exit-pipe.sh (#402).
#
# The script under test scans a fixed set of paths under the repository root, so
# each case builds a throwaway tree with that layout and runs the script from
# inside it. That keeps the cases independent of whatever the real workflows
# happen to contain today — a suite that asserted "the repository is clean" would
# start failing for reasons that have nothing to do with the checker.
set -uo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
script="${here}/no-early-exit-pipe.sh"

failures=0
pass() { printf '  ✅ %s\n' "$1"; }
fail() { printf '  ❌ %s\n' "$1"; failures=$((failures + 1)); }

# run_case <workflow-content> -> prints exit code and output
run_case() {
  local content="$1" dir
  dir="$(mktemp -d)"
  mkdir -p "${dir}/.github/workflows" "${dir}/.github/scripts"
  cp "$script" "${dir}/.github/scripts/"
  printf '%s\n' "$content" > "${dir}/.github/workflows/sample.yml"
  ( cd "$dir" && .github/scripts/no-early-exit-pipe.sh 2>&1 )
  local rc=$?
  rm -rf "$dir"
  return $rc
}

expect_clean() {
  local label="$1" content="$2" out rc
  out="$(run_case "$content")"; rc=$?
  if [ "$rc" -ne 0 ]; then
    fail "$label — expected exit 0, got $rc: $out"
  elif ! grep -q 'no pipeline in' <<<"$out"; then
    # The success LINE, with its file count — a checker that scanned nothing at
    # all would also exit 0, which is the failure mode this repository keeps
    # finding in its own guards.
    fail "$label — exit 0 without the success line: $out"
  else
    pass "$label"
  fi
}

expect_flagged() {
  local label="$1" content="$2" out rc
  out="$(run_case "$content")"; rc=$?
  if [ "$rc" -eq 0 ]; then
    fail "$label — expected a violation, got exit 0: $out"
  else
    pass "$label"
  fi
}

echo "flagged shapes"
expect_flagged "printf into grep -q"        "        run: |
          if ! printf '%s' \"\$x\" | grep -q foo; then exit 1; fi"
expect_flagged "cat into grep -qi"          "        run: |
          cat f | grep -qi foo"
expect_flagged "producer into grep -Eq"     "        run: |
          echo \"\$x\" | grep -Eq '^a+\$'"
expect_flagged "producer into grep --quiet" "        run: |
          echo \"\$x\" | grep --quiet foo"
expect_flagged "producer into head"         "        run: |
          printf '%s' \"\$x\" | head -20"

echo "accepted shapes"
expect_clean "a here-string" "        run: |
          if ! grep -q foo <<<\"\$x\"; then exit 1; fi"
expect_clean "a pipe into awk, which reads to EOF" "        run: |
          needs=\"\$(printf '%s\\n' \"\$block\" | awk '/needs/ { print }')\""
expect_clean "a pipe into sed or wc" "        run: |
          n=\"\$(printf '%s' \"\$x\" | wc -l)\"
          y=\"\$(printf '%s' \"\$x\" | sed 's/a/b/')\""
expect_clean "grep without -q, whose reader drains the pipe" "        run: |
          printf '%s' \"\$x\" | grep foo"
expect_clean "the shape named inside a comment" "        run: |
          # do not write: printf '%s' \"\$x\" | grep -q foo
          grep -q foo <<<\"\$x\""

# A trailing comment must not launder a real occurrence on the same line.
expect_flagged "code with a trailing comment" "        run: |
          printf '%s' \"\$x\" | grep -q foo   # still racy"

if [ "$failures" -ne 0 ]; then
  echo "${failures} case(s) failed"
  exit 1
fi
echo "all cases passed"
