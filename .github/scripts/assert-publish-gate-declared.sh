#!/usr/bin/env bash
#
# Assert docker-publish.yml still declares its dependency on a suite run (#356,
# #359). Extracted from the inline ci.yml step in #402 so both directions can be
# tested: the case that broke was the guard reporting a violation on a tree that
# had none, and an inline step can only be exercised by pushing a commit.
#
# Takes the workflow to inspect as $1 so the tests can point it at stubs; the
# script has no knowledge of being tested, and CI runs the same bytes.
#
# 🚨 No `printf … | grep -q` in here, deliberately. That shape is what #402 was:
# `grep -q` exits 0 the instant it matches, closing the pipe under the writer's
# feet; the writer dies on EPIPE, `pipefail` adopts its status, and a leading `!`
# turns the match into the failure branch. So the guard failed BECAUSE the
# pattern it was looking for was present. Here-strings have no second process to
# race with. See the shape check in ci.yml that keeps this from coming back.
set -euo pipefail

wf="${1:-.github/workflows/docker-publish.yml}"
gate=require-suite
script=.github/scripts/require-suite-run.sh

if [ ! -f "$wf" ]; then
  echo "no workflow file at $wf (#356): this guard cannot inspect what it guards"
  exit 1
fi

# Isolate one job: from its `  <id>:` header to the next one, scanning only
# after `jobs:`. Job-level keys sit at 4 spaces and step bodies deeper, so a
# bare 2-space `key:` inside a job cannot end a block early.
job_block() {
  awk -v want="  $1:" '
    /^jobs:$/                     { injobs = 1; next }
    !injobs                       { next }
    /^  [A-Za-z0-9_-]+:$/         { inblock = ($0 == want) }
    inblock                       { print }
  ' "$wf"
}

gate_block="$(job_block "$gate")"
if [ -z "$gate_block" ]; then
  echo "no '$gate' job in $wf (#356): the job that requires a green suite run for the commit being published is gone, or was renamed without updating this check"
  exit 1
fi

publish_block="$(job_block publish)"
if [ -z "$publish_block" ]; then
  echo "no 'publish' job in $wf (#356): renamed? this check can no longer find what it guards"
  exit 1
fi

# Collect the needs declaration in EITHER YAML form: inline on the key
# (`needs: x`, `needs: [x, y]`) or a block sequence beneath it. Matching only
# the inline form would report the dependency as GONE after a purely
# cosmetic reformat — a false alarm that says "publishing is ungated" about a
# file where it is still gated, which is worse than silence because someone
# would go looking for a break that is not there.
#
# The pipe into awk is safe where a pipe into `grep -q` is not: awk reads to
# EOF, so it never closes the pipe early and the writer never sees EPIPE.
needs_decl="$(printf '%s\n' "$publish_block" | awk '
  /^    needs:/                { inneeds = 1; print; next }
  inneeds && /^      *[-#]/    { print; next }
  inneeds                      { inneeds = 0 }
')"

if [ -z "$needs_decl" ]; then
  echo "the publish job in $wf declares no 'needs:' at all (#356): images would publish from a commit with no suite run again, and every check in this repository would stay green while it happened"
  exit 1
fi

# Word boundaries by hand: `require-suite` must not be satisfied by a
# longer id that merely contains it.
if ! grep -Eq "(^|[^A-Za-z0-9_-])${gate}([^A-Za-z0-9_-]|$)" <<<"$needs_decl"; then
  echo "the publish job in $wf no longer needs '$gate' (#356): images would publish from a commit with no suite run again, and every check in this repository would stay green while it happened"
  exit 1
fi

# The gate must still run the script. Without this the job could survive as
# an empty shell that satisfies the needs edge above and gates nothing.
#
# ⚠️ The two content checks below moved here from the job block when the
# loop was extracted (#359). They read the SCRIPT because that is where the
# text now lives; left pointing at the job block they would have passed
# trivially forever, since the block no longer contains either string —
# a guard that cannot fail, in the file whose whole subject is guards that
# cannot fail.
if ! grep -q "$script" <<<"$gate_block"; then
  echo "the '$gate' job in $wf no longer runs $script (#356, #359): the needs edge would still be declared while the gate asserts nothing"
  exit 1
fi

if [ ! -x "$script" ]; then
  echo "$script is missing or not executable (#359): the gate job would fail on every publish, or worse, be quietly edited to inline something else"
  exit 1
fi

if ! grep -q 'workflows/ci\.yml/runs' "$script"; then
  echo "$script no longer looks up a ci.yml run (#356): the needs edge would still be declared while the gate asserts nothing"
  exit 1
fi

# web.yml is path-filtered and legitimately does not run on most commits, so
# waiting for one of its runs would hang until timeout and block publishing
# from any commit that did not touch web/ — the never-reports-so-never-passes
# shape that keeps `web` out of the required checks on main. Catch it as a
# text change here rather than as a stuck release later.
if grep -q 'workflows/web\.yml/runs' "$script"; then
  echo "$script waits on a web.yml run (#356): web.yml is path-filtered, so on a commit that touches no web path that run never exists and the gate would block the publish forever"
  exit 1
fi

# 🚨 Both jobs must check out the commit the gate verified. require-suite
# proves a green suite run exists for github.sha; the publish checkout
# decides which tree is built. If those ever resolved differently, the gate
# would pass on one commit while the image came from another and the whole
# guarantee would be false with every check green. The default ref happens
# to resolve correctly for the triggers this file declares today, which is
# a coincidence of the trigger list rather than a property — so it is
# pinned, and asserted here so a later edit cannot quietly unpin it.
for job in "$gate" publish; do
  if ! grep -q 'ref: .*github\.sha' <<<"$(job_block "$job")"; then
    echo "the '$job' job in $wf checks out without pinning ref to github.sha (#356): the gate proves a run exists for that commit, and an unpinned checkout lets a later trigger build a different tree while the gate still passes"
    exit 1
  fi
done

echo "publish gate is declared: publish needs $gate, $gate runs $script, which consults ci.yml and does not wait on path-filtered web.yml; both jobs check out github.sha"
