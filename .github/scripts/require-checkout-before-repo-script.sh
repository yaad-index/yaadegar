#!/usr/bin/env bash
#
# Fail when a workflow job runs a script from the repository without checking the
# repository out first (#378).
#
# 🚨 WHAT THIS PREVENTS, stated so the name cannot be read as more than it is.
# #368 added a step to release-please.yml that runs a script from the working tree:
#
#     - name: Request reviewers on the release pull request
#       if: steps.release.outputs.prs_created == 'true'
#       run: .github/scripts/request-pr-reviewers.sh
#
# Its job had no checkout step. A runner's workspace starts empty, so that path did
# not exist and the step could not run. It took until #377 to notice, and the reason
# it took that long is the part worth encoding: the step is conditional on a release
# pull request having been cut, none had been cut since it shipped, and **a skipped
# step reports success**. The workflow was green on every run while carrying a step
# that could never have worked.
#
# That is the failure this check exists for — not a broken run anyone saw, but a
# green one that proved nothing. The sibling caller in docs-compose.yml checked out
# before running the same script, so the repository looked consistent from the outside.
#
# ⚠️ WHAT THIS DOES *NOT* DO. The class is "a step depends on repository content that
# nothing put in the workspace". This check covers exactly one member of it:
#
#   the literal string `.github/scripts/` appearing on a non-comment line of a job.
#
# That is the one form this repository uses and the one that has actually broken. It
# will not notice a script invoked through a variable, reached via `working-directory`,
# named by a bare relative path, or any other repository file a step reads — a config,
# a fixture, a Makefile. **Read a pass as "no job invokes .github/scripts/ without a
# checkout", never as "every step has the files it needs".**
#
# ⚠️ Nor does it verify the checkout is useful: `actions/checkout` with a `ref`,
# `sparse-checkout`, or a path filter that excludes the script still counts here. The
# check is positional, and says so.
#
# ⚠️ FALSE POSITIVES ARE THE CHOSEN DIRECTION, deliberately. A job that produces a
# file at that path before running it, or that populates the workspace by some means
# other than `actions/checkout`, is reported as a finding. Both are rare; a noisy
# failure is argued about once and fixed, while a silent miss is what produced the
# defect this check is named after. Being wrong loudly is the trade, not an oversight.
#
# ⚠️ Ordering is checked, not merely presence. A checkout that runs AFTER the step
# invoking the script leaves that step with nothing, so a job whose checkout appears
# below the invocation is a finding even though both are present.
#
# 🚨 COMMENTS ARE SKIPPED, and that is load-bearing rather than tidiness. A matcher
# scanning prose cannot tell use from mention, and prose ABOUT a rule is the densest
# possible source of that rule's trigger tokens: the workflow comments explaining this
# very defect quote the path that trips it. Flagging them would train people to stop
# writing the explanation, which is the opposite of what is wanted.
#
# Usage: require-checkout-before-repo-script.sh [file ...]
#        (default: .github/workflows/*.yml and *.yaml)
set -euo pipefail

# The literal form this check knows about, named once so the message and the scan
# cannot drift apart.
readonly NEEDLE='.github/scripts/'

files=("$@")
if [ "${#files[@]}" -eq 0 ]; then
  # Globbed here rather than by the caller so CI invokes it with no arguments and
  # cannot silently pass by supplying an empty list.
  shopt -s nullglob
  files=(.github/workflows/*.yml .github/workflows/*.yaml)
  shopt -u nullglob
fi

if [ "${#files[@]}" -eq 0 ]; then
  echo "no workflow files found to scan (#378): a check that examines nothing and reports success is worse than no check"
  exit 1
fi

findings=0
scanned=0
jobs_seen=0

for wf in "${files[@]}"; do
  [ -f "$wf" ] || { echo "not a file: $wf"; exit 1; }
  scanned=$(( scanned + 1 ))

  # Job boundaries are found by indentation rather than by parsing YAML, for the same
  # reason the sibling check scans lines: a hand-rolled block parser is exactly the
  # part fiddly enough to go wrong quietly. The cost is that this understands one
  # layout — `jobs:` at column 0, job ids two spaces in — so a file it cannot read
  # is an ERROR below rather than a pass.
  in_jobs=0
  job=''
  job_line=0
  hit_line=0      # first non-comment line in this job mentioning the needle
  checkout_line=0 # first non-comment `uses: actions/checkout@` in this job
  file_jobs=0

  flush() { # evaluate the job that just ended
    [ -n "$job" ] || return 0
    if [ "$hit_line" -ne 0 ] && { [ "$checkout_line" -eq 0 ] || [ "$checkout_line" -gt "$hit_line" ]; }; then
      findings=$(( findings + 1 ))
      if [ "$checkout_line" -eq 0 ]; then
        echo "${wf}:${hit_line}: job '${job}' (line ${job_line}) runs ${NEEDLE}… but never checks the repository out"
        echo "    a runner's workspace starts empty, so that path does not exist when the step runs"
      else
        echo "${wf}:${hit_line}: job '${job}' (line ${job_line}) runs ${NEEDLE}… before its checkout (line ${checkout_line})"
        echo "    the workspace is still empty at that point, so being present later does not help"
      fi
      echo "    add: - uses: actions/checkout@<pinned-sha>   above it"
    fi
  }

  lineno=0
  while IFS= read -r line || [ -n "$line" ]; do
    lineno=$(( lineno + 1 ))

    # See the comment note in the header: mention is not use.
    trimmed="${line#"${line%%[![:space:]]*}"}"
    case "$trimmed" in '#'*) continue ;; esac

    case "$line" in
      'jobs:'*) in_jobs=1; continue ;;
    esac

    if [ "$in_jobs" -eq 1 ]; then
      # A job id: exactly two spaces, a key, nothing else of substance on the line.
      if [[ "$line" =~ ^\ \ ([A-Za-z0-9_.-]+):[[:space:]]*$ ]]; then
        flush
        job="${BASH_REMATCH[1]}"
        job_line=$lineno
        hit_line=0
        checkout_line=0
        file_jobs=$(( file_jobs + 1 ))
        continue
      fi
      # A top-level key at column 0 ends the jobs block.
      if [[ "$line" =~ ^[A-Za-z] ]]; then
        flush
        job=''
        in_jobs=0
        continue
      fi
    fi

    [ -n "$job" ] || continue

    if [ "$hit_line" -eq 0 ] && [[ "$line" == *"$NEEDLE"* ]]; then
      hit_line=$lineno
    fi
    if [ "$checkout_line" -eq 0 ] && [[ "$line" =~ uses:[[:space:]]*actions/checkout@ ]]; then
      checkout_line=$lineno
    fi
  done < "$wf"
  flush

  # A workflow file with no job this can see means the layout assumption above did not
  # hold. Reporting success there would be the vacuous pass this check exists to avoid,
  # so it is an error that names the file.
  if [ "$file_jobs" -eq 0 ]; then
    echo "${wf}: found no jobs to examine (#378): this check reads \`jobs:\` at column 0 with job ids indented two spaces, and this file does not match — it has NOT been checked"
    exit 1
  fi
  jobs_seen=$(( jobs_seen + file_jobs ))
done

if [ "$findings" -ne 0 ]; then
  echo
  echo "${findings} job(s) run a repository script with no checkout in front of it. A skipped step reports SUCCESS, so a job like this stays green until the condition that runs the step finally arrives — see #377."
  exit 1
fi

echo "no job runs ${NEEDLE}… without a preceding checkout (${jobs_seen} job(s) across ${scanned} workflow file(s))"
