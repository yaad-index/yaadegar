#!/usr/bin/env bash
#
# Tests for require-suite-run.sh (#359).
#
# The script polls the Actions API, so the API is stubbed: a fake `gh` is put
# earlier on PATH and reads its scripted answers from STUB_REPLIES. The script
# under test has no knowledge of any of this — production runs the same bytes.
#
# 🚨 These assert the TICK LINES, not only the exit status and the final message.
# The defect this file exists for was log-only and on a tick that was NOT the last
# one: the per-tick echo recomputed its text from $status instead of consuming
# tick_note, so a tolerated lookup failure reported the last successful
# observation as though it were current. Exit status was correct throughout and
# the final message was correct throughout. A test that checks only those two
# passes with the bug present.
#
# ⚠️ The failure branch of that line is also the branch that has never run in
# production: the gate's live runs have had every lookup succeed. An unopposed
# negative — no wrong tick lines seen, in runs where nothing could produce one —
# is much weaker evidence than it reads as, so the stub drives that branch here.
set -uo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
script="${here}/require-suite-run.sh"

SHA=0000000000000000000000000000000000000000
failures=0

# run_case executes the script against a scripted sequence of `gh` answers.
#
# STUB_REPLIES holds one reply per call, separated by '|'. A reply of "fail" makes
# the stub exit non-zero, standing in for an API blip; anything else is echoed
# verbatim and is expected to be the tab-separated status/conclusion/url triple the
# real --jq template produces. Running past the end of the list repeats the last
# reply, so a case does not have to predict how many ticks it will take.
run_case() {
  local replies="$1" wait_seconds="$2"
  local dir; dir="$(mktemp -d)"

  cat > "${dir}/gh" <<'STUB'
#!/usr/bin/env bash
count_file="${STUB_COUNT_FILE}"
n=$(cat "$count_file" 2>/dev/null || echo 0)
echo $(( n + 1 )) > "$count_file"

IFS='|' read -r -a replies <<< "${STUB_REPLIES}"
index=$n
if [ "$index" -ge "${#replies[@]}" ]; then
  index=$(( ${#replies[@]} - 1 ))
fi
reply="${replies[$index]}"

if [ "$reply" = "fail" ]; then
  echo "stub: pretending the API call failed" >&2
  exit 1
fi
printf '%s\n' "$reply"
STUB
  chmod +x "${dir}/gh"

  STUB_REPLIES="$replies" \
  STUB_COUNT_FILE="${dir}/count" \
  PATH="${dir}:${PATH}" \
  REPO=owner/repo \
  SHA="$SHA" \
  WAIT_SECONDS="$wait_seconds" \
  POLL_SECONDS=0 \
  GH_TOKEN=stub \
    bash "$script"
  local code=$?

  rm -rf "$dir"
  return $code
}

# assert_contains fails the run rather than aborting it, so one broken expectation
# does not hide the state of every case after it.
assert_contains() {
  local haystack="$1" needle="$2" what="$3"
  if [[ "$haystack" == *"$needle"* ]]; then
    echo "  ok: ${what}"
  else
    echo "  FAIL: ${what}"
    echo "    wanted to find: ${needle}"
    echo "    in:"
    printf '      %s\n' "$haystack"
    failures=$(( failures + 1 ))
  fi
}

# assert_count is what the regression case needs. "Does the wrong text appear" is
# not the question — with the bug the stale status line appears a SECOND time,
# alongside a correct one, so a contains/not-contains pair cannot separate them.
assert_count() {
  local haystack="$1" needle="$2" want="$3" what="$4"
  local got
  got="$(printf '%s\n' "$haystack" | grep -c -F -- "$needle" || true)"
  if [ "$got" -eq "$want" ]; then
    echo "  ok: ${what}"
  else
    echo "  FAIL: ${what} (found ${got}, wanted ${want})"
    echo "    needle: ${needle}"
    echo "    in:"
    printf '      %s\n' "$haystack"
    failures=$(( failures + 1 ))
  fi
}

assert_not_contains() {
  local haystack="$1" needle="$2" what="$3"
  if [[ "$haystack" != *"$needle"* ]]; then
    echo "  ok: ${what}"
  else
    echo "  FAIL: ${what}"
    echo "    did not want: ${needle}"
    failures=$(( failures + 1 ))
  fi
}

assert_status() {
  local got="$1" want="$2" what="$3"
  if [ "$got" -eq "$want" ]; then
    echo "  ok: ${what}"
  else
    echo "  FAIL: ${what} (exit ${got}, wanted ${want})"
    failures=$(( failures + 1 ))
  fi
}

echo "a run that has already succeeded is published immediately"
out="$(run_case 'completed	success	https://example.test/run/1' 60)"; code=$?
assert_status "$code" 0 "exits 0"
assert_contains "$out" "succeeded: https://example.test/run/1" "names the run it passed on"

echo "queued then in progress then success: every NON-TERMINAL tick says what that tick saw"
out="$(run_case 'queued		|in_progress		|completed	success	https://example.test/run/2' 60)"; code=$?
assert_status "$code" 0 "exits 0"
assert_contains "$out" "suite run for ${SHA} is 'queued'; waiting 0s" "the queued tick"
assert_contains "$out" "suite run for ${SHA} is 'in_progress'; waiting 0s" "the in-progress tick"
assert_contains "$out" "succeeded" "and then it publishes"

echo "a tolerated blip AFTER a successful observation reports the LOOKUP, not the stale status"
# 🚨 This is the regression case. With the tick line recomputed from \$status, the
# second tick would read "is 'queued'" — the previous observation, presented as
# current — and every other assertion in this file would still pass.
out="$(run_case 'queued		|fail|completed	success	https://example.test/run/3' 60)"; code=$?
assert_status "$code" 0 "a blip does not fail the gate"
assert_contains "$out" "the Actions API lookup failed (1 so far); this is the lookup, not a verdict on the suite" "the blip tick names itself as a lookup failure"
assert_count "$out" "is 'queued'; waiting 0s" 1 "the queued status is reported ONCE, not repeated on the blip tick"

echo "a blip on the FIRST tick does not claim the run does not exist"
out="$(run_case 'fail|completed	success	https://example.test/run/4' 60)"; code=$?
assert_status "$code" 0 "exits 0 once the lookup recovers"
assert_contains "$out" "the Actions API lookup failed (1 so far)" "says the lookup failed"
assert_not_contains "$out" "not created yet" "does not assert absence from a tick that learned nothing"

echo "a run that concluded failure is refused without waiting"
out="$(run_case 'completed	failure	https://example.test/run/5' 60)"; code=$?
assert_status "$code" 1 "exits 1"
assert_contains "$out" "concluded 'failure', not success" "names the conclusion"

echo "a run still unfinished at the deadline is refused, and says which state it was in"
out="$(run_case 'in_progress		https://example.test/run/6' 0)"; code=$?
assert_status "$code" 1 "exits 1"
assert_contains "$out" "was still 'in_progress' after 0s" "reports the last observed state"

echo "no run at all is refused with the absence message, which points at the trigger"
out="$(run_case '		' 0)"; code=$?
assert_status "$code" 1 "exits 1"
assert_contains "$out" "no ci.yml run exists" "says the run is missing"
assert_contains "$out" "absence is not evidence of a pass" "and refuses to read absence as permission"

echo "lookups that never succeed are refused as a LOOKUP failure, never as a missing run"
# The two are opposite facts and the messages must not be interchangeable: one
# says the suite's state is unknown, the other asserts a run does not exist.
out="$(run_case 'fail' 0)"; code=$?
assert_status "$code" 1 "exits 1"
assert_contains "$out" "could not reach the Actions API" "names the lookup as the thing that broke"
assert_contains "$out" "the suite's actual state is unknown" "and refuses to state what it never observed"
assert_not_contains "$out" "no ci.yml run exists" "does not claim absence it never established"

echo "an abbreviated SHA is refused as malformed INPUT, not reported as a missing run"
# 🚨 The two are opposite diagnoses and the API cannot tell them apart: an
# abbreviated SHA returns the same empty list a commit with no run returns. Without
# the length check the gate fails under the missing-run message, which sends the
# reader to inspect a trigger that is working.
out="$(SHA=abc1234 run_case 'completed	success	https://example.test/run/9' 60)"; code=$?
assert_status "$code" 1 "exits 1"
assert_contains "$out" "must be the full 40-character commit" "names the input as the problem"
assert_not_contains "$out" "no ci.yml run exists" "does not blame a trigger that is fine"

echo
if [ "$failures" -ne 0 ]; then
  echo "${failures} assertion(s) failed"
  exit 1
fi
echo "all assertions passed"
