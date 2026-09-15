#!/usr/bin/env bash
#
# Tests for assert-publish-gate-declared.sh (#402).
#
# 🚨 The FIRST case is the point of this file. #402 was the guard failing on a
# tree that satisfied it — a false violation, not a missed one — and every
# negative case below would have passed throughout that bug. A suite made only of
# "it rejects a stub with the pin removed" stays green while the guard is broken
# in the direction that actually bit.
#
# The second case is the regression pin for the mechanism itself. The old guard
# piped its writer into `grep -q`; `grep -q` exits the instant it matches, the
# writer dies on EPIPE, `pipefail` adopts that status and `!` inverts it into the
# failure branch — so the check failed BECAUSE the pattern was present, and more
# often the larger the block. Padding a valid workflow past the pipe buffer makes
# that deterministic rather than a race: the old form fails this case every time,
# the here-string form passes at any size.
set -uo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo="$(cd "${here}/../.." && pwd)"
script="${here}/assert-publish-gate-declared.sh"

cd "$repo"
failures=0

pass() { printf '  ✅ %s\n' "$1"; }
fail() { printf '  ❌ %s\n' "$1"; failures=$((failures + 1)); }

# expect_ok <label> <workflow-file>
expect_ok() {
  local label="$1" wf="$2" out rc
  out="$("$script" "$wf" 2>&1)"; rc=$?
  if [ "$rc" -ne 0 ]; then
    fail "$label — expected exit 0, got $rc: $out"
  elif ! grep -q 'publish gate is declared' <<<"$out"; then
    # The success LINE, not just the status: a guard that silently did nothing
    # would also exit 0, and that is the shape this whole file is about.
    fail "$label — exit 0 but no success line: $out"
  else
    pass "$label"
  fi
}

# expect_violation <label> <workflow-file> <expected substring>
expect_violation() {
  local label="$1" wf="$2" want="$3" out rc
  out="$("$script" "$wf" 2>&1)"; rc=$?
  if [ "$rc" -eq 0 ]; then
    fail "$label — expected a violation, got exit 0: $out"
  elif ! grep -qF "$want" <<<"$out"; then
    # Assert WHICH violation: several of these exit 1 for different reasons, and a
    # test that only counted the exit code would accept the wrong diagnosis.
    fail "$label — wrong message. wanted substring: $want / got: $out"
  else
    pass "$label"
  fi
}

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

# A minimal workflow that satisfies every property the guard checks.
good() {
  cat <<'YAML'
name: docker-publish
jobs:
  require-suite:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1
        with:
          ref: ${{ github.sha }}
      - name: Require a green suite run
        run: .github/scripts/require-suite-run.sh
  publish:
    needs: require-suite
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1
        with:
          ref: ${{ github.sha }}
YAML
}

echo "positive cases"

# 1. The real tree. This is the case #402 broke, and the one a stub cannot stand
#    in for: the guard is pointed at the file it actually guards.
expect_ok "the guard passes on the current tree" .github/workflows/docker-publish.yml

# 2. The #402 regression pin: same valid content, padded past the pipe buffer.
{
  good
  echo "      # padding to force the writer past the 64KiB pipe buffer:"
  # Indented deeper than 4 spaces so the padding stays inside the publish block.
  for _ in $(seq 1 1200); do
    echo "          # xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
  done
} > "${tmp}/padded.yml"
expect_ok "a valid workflow still passes when the publish block exceeds the pipe buffer (#402)" "${tmp}/padded.yml"

echo "negative cases"

good | sed 's/^          ref: ${{ github.sha }}$/          fetch-depth: 0/' > "${tmp}/unpinned.yml"
expect_violation "an unpinned checkout is refused" "${tmp}/unpinned.yml" \
  "checks out without pinning ref to github.sha"

good | grep -v '^    needs: require-suite$' > "${tmp}/no-needs.yml"
expect_violation "a publish job with no needs: is refused" "${tmp}/no-needs.yml" \
  "declares no 'needs:' at all"

good | sed 's/^    needs: require-suite$/    needs: some-other-job/' > "${tmp}/wrong-needs.yml"
expect_violation "a publish job needing something else is refused" "${tmp}/wrong-needs.yml" \
  "no longer needs 'require-suite'"

good | sed 's|^        run: .github/scripts/require-suite-run.sh$|        run: echo gated|' > "${tmp}/hollow-gate.yml"
expect_violation "a gate job that no longer runs the script is refused" "${tmp}/hollow-gate.yml" \
  "no longer runs .github/scripts/require-suite-run.sh"

good | sed 's/^  publish:$/  publish-renamed:/' > "${tmp}/no-publish.yml"
expect_violation "a missing publish job is refused" "${tmp}/no-publish.yml" \
  "no 'publish' job"

good | sed 's/^  require-suite:$/  require-suite-renamed:/' > "${tmp}/no-gate.yml"
expect_violation "a missing gate job is refused" "${tmp}/no-gate.yml" \
  "no 'require-suite' job"

expect_violation "a missing workflow file is refused" "${tmp}/does-not-exist.yml" \
  "no workflow file at"

# A longer id that merely contains the gate name must not satisfy the needs edge.
good | sed 's/^    needs: require-suite$/    needs: require-suite-lite/' > "${tmp}/substring-needs.yml"
expect_violation "a longer job id containing the gate name does not satisfy it" "${tmp}/substring-needs.yml" \
  "no longer needs 'require-suite'"

if [ "$failures" -ne 0 ]; then
  echo "${failures} case(s) failed"
  exit 1
fi
echo "all cases passed"
