#!/usr/bin/env bash
#
# Wait for the ci.yml run belonging to one commit and report whether it passed.
#
# This is the body of docker-publish.yml's require-suite job, kept in a file so it
# can be run against a stub `gh` and asserted (#359). It was previously inline in
# the workflow, where nothing could invoke it: the runs that established it worked
# were done by hand while writing it, and evidence about one revision is not
# coverage.
#
# It takes no arguments. Everything is environment, exactly as the job supplied it:
#
#   REPO           owner/name to query
#   SHA            the full 40-character commit SHA being published
#   WAIT_SECONDS   how long to keep waiting before giving up
#   POLL_SECONDS   pause between ticks
#   GH_TOKEN       consumed by `gh` itself
#
# ⚠️ SHA must be the FULL 40 characters. The Actions API does not match an
# abbreviated SHA — it returns an empty list, which is indistinguishable from "no
# run exists" and would be reported here as the missing-run failure below. That is
# the alarm state, produced by malformed input.
#
# `gh` is invoked by name so a test can put a stub earlier on PATH. There is
# deliberately no indirection for it in this script: production runs the same bytes
# the test runs, with nothing switched off.
set -euo pipefail

: "${REPO:?REPO must name the repository to query}"
: "${SHA:?SHA must be the full 40-character commit being published}"
: "${WAIT_SECONDS:?WAIT_SECONDS must bound the wait}"
: "${POLL_SECONDS:?POLL_SECONDS must set the tick interval}"

deadline=$(( $(date +%s) + WAIT_SECONDS ))
seen_run=no
lookups_ok=0
lookup_failures=0

while :; do
  # Newest run for this exact commit. Re-runs update a run in place, so the latest
  # is the current verdict rather than one of several opinions.
  # No event filter: a run's evidence is about the tree at that SHA, and which
  # event started it does not change what was executed — filtering to `push` would
  # only add a way to reject evidence that exists.
  #
  # A failed lookup is not a verdict. Under `set -e` a single blip in one of the
  # ~135 calls this wait can make would end the job, and it would surface as a red
  # gate with no run URL — indistinguishable, to the next reader, from "the suite
  # failed", which is the opposite of what happened. So tolerate the call and try
  # again on the next tick.
  #
  # Tolerated, NOT ignored: a lookup that never once succeeds still fails at the
  # deadline below, under its own message. Retrying is what keeps a transient blip
  # from becoming a verdict; the counters are what keep a permanent break from
  # quietly becoming a pass.
  if info="$(gh api \
    "repos/${REPO}/actions/workflows/ci.yml/runs?head_sha=${SHA}&per_page=100" \
    --jq '.workflow_runs | sort_by(.run_started_at) | last
          | "\(.status // "")\t\(.conclusion // "")\t\(.html_url // "")"')"; then
    lookups_ok=$(( lookups_ok + 1 ))

    status="$(printf '%s' "$info" | cut -f1)"
    conclusion="$(printf '%s' "$info" | cut -f2)"
    url="$(printf '%s' "$info" | cut -f3)"

    if [ -n "$status" ]; then
      seen_run=yes
    fi

    if [ "$status" = "completed" ]; then
      if [ "$conclusion" = "success" ]; then
        echo "suite run for ${SHA} succeeded: ${url}"
        exit 0
      fi
      # A finished non-success will not improve by waiting.
      echo "the suite run for ${SHA} concluded '${conclusion}', not success (#356): refusing to publish an image from a commit whose tests did not pass — ${url}"
      exit 1
    fi

    tick_note="suite run for ${SHA} is '${status:-not created yet}'"
  else
    # Do NOT overwrite status/url here, and do not loop straight back.
    #
    # Not overwriting: those variables hold the last thing actually observed. A
    # blip on the final tick would otherwise blank them and make the timeout below
    # report a run as still '' — a state no API ever returned.
    #
    # Not short-circuiting: the deadline check below is the one place this loop can
    # end, so jumping past it would let a permanently broken lookup spin until the
    # runner timeout killed the job, giving a red with no message at all — the
    # failure being fixed here, only worse.
    lookup_failures=$(( lookup_failures + 1 ))
    tick_note="the Actions API lookup failed (${lookup_failures} so far); this is the lookup, not a verdict on the suite"
  fi

  if [ "$(date +%s)" -ge "$deadline" ]; then
    # The two timeouts are different failures and must not read alike.
    # An absent run is the dangerous one: it is what a deleted push trigger, a
    # renamed workflow file, or a run that never dispatched all look like, and
    # treating absence as permission is exactly the hole this job exists to close.
    if [ "$seen_run" = yes ]; then
      echo "the suite run for ${SHA} was still '${status}' after ${WAIT_SECONDS}s (#356): not publishing on an unfinished run — ${url}"
    elif [ "$lookups_ok" -eq 0 ]; then
      # Third case, and it must not read like either of the others: nothing was
      # ever learned about the suite. Saying "no run exists" here would state a
      # fact that was never observed.
      echo "could not reach the Actions API for ${SHA}: all ${lookup_failures} lookups failed over ${WAIT_SECONDS}s (#356) — this is the lookup breaking, NOT the suite failing and NOT a missing run; the suite's actual state is unknown"
    else
      echo "no ci.yml run exists for ${SHA} after ${WAIT_SECONDS}s (#356): the push trigger on ci.yml is what creates it, so check that it is still there and that the workflow file is still named ci.yml — absence is not evidence of a pass"
    fi
    exit 1
  fi

  # Report what THIS tick learned. Recomputing the line from $status here would
  # print the last successful observation as though it were current on a tolerated
  # failure, and would claim "not created yet" when the very first lookup failed —
  # asserting a run does not exist on a tick that learned nothing about whether it
  # exists, which is the same absence-is-not-evidence error the deadline branch is
  # careful to avoid.
  echo "${tick_note}; waiting ${POLL_SECONDS}s"
  sleep "$POLL_SECONDS"
done
