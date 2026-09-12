#!/usr/bin/env bash
#
# Fail when a workflow uses `fromJSON(X)` without short-circuiting it on X (#375).
#
# 🚨 WHAT THIS PREVENTS, stated so the name cannot be read as more than it is.
# #373 shipped this inside a step's `env:` block:
#
#     PR_NUMBER: ${{ fromJSON(steps.release.outputs.pr).number }}
#
# guarded by `if: steps.release.outputs.prs_created == 'true'`. That output is an
# EMPTY STRING on any push to main with no releasable commits, `fromJSON('')`
# throws, and **a step's `env:` block is evaluated even when the step is skipped**
# — so the `if:` guarded nothing and the job failed. Most pushes are that case.
#
# The fix is to short-circuit on the same operand, because `&&` returns the first
# falsy operand without evaluating the second:
#
#     PR_NUMBER: ${{ steps.release.outputs.pr && fromJSON(steps.release.outputs.pr).number }}
#
# ⚠️ WHAT THIS DOES *NOT* DO. The class is "an expression that can throw, evaluated
# in a context an `if:` does not protect". This check covers exactly one member of
# it — the literal `fromJSON(` — because that is the one the repository uses and the
# one that has actually broken a run. It will not notice some other throwing
# expression invented later. Read a pass as "no unguarded fromJSON", never as "no
# throwing expression".
#
# ⚠️ It is deliberately BROADER than the verified hazard in one direction: it flags
# an unguarded `fromJSON` anywhere in a workflow, not only inside `env:`. Measured
# on a live runner, a skipped step evaluates its `env:` but NOT its `with:`, `run:`
# or `name:`, so only `env:` can fail this way today. Scanning every line keeps the
# check free of YAML block tracking, which is the part that would make it fiddly
# enough to pass vacuously — and the guard is harmless wherever it appears. The cost
# is that it may ask for a guard that is not strictly needed; that trade is the
# point, not an oversight.
#
# ⚠️ Known blind spot: the argument is matched with `[^)]*`, so a `fromJSON` whose
# argument itself contains a parenthesis is not parsed correctly and may be reported
# as unguarded. No occurrence in this repository has one. A false positive here is
# loud and fixable; that is the right direction for this check to be wrong.
#
# Usage: no-throwing-env-expr.sh [file ...]   (default: .github/workflows/*.yml)
set -euo pipefail

files=("$@")
if [ "${#files[@]}" -eq 0 ]; then
  # Globbed here rather than by the caller so CI invokes it with no arguments and
  # cannot silently pass by supplying an empty list.
  shopt -s nullglob
  files=(.github/workflows/*.yml .github/workflows/*.yaml)
  shopt -u nullglob
fi

if [ "${#files[@]}" -eq 0 ]; then
  echo "no workflow files found to scan (#375): a check that examines nothing and reports success is worse than no check"
  exit 1
fi

# Strip every space so `X &&`, `X&&` and `X  &&` compare alike.
squeeze() { printf '%s' "${1// /}"; }

findings=0
scanned=0

for wf in "${files[@]}"; do
  [ -f "$wf" ] || { echo "not a file: $wf"; exit 1; }
  scanned=$(( scanned + 1 ))
  lineno=0
  while IFS= read -r line || [ -n "$line" ]; do
    lineno=$(( lineno + 1 ))

    # Skip comment-only lines. The explanatory comments in this repository quote the
    # broken form on purpose, and flagging prose would train people to stop writing it.
    trimmed="${line#"${line%%[![:space:]]*}"}"
    case "$trimmed" in '#'*) continue ;; esac

    case "$line" in *'fromJSON('*) ;; *) continue ;; esac

    rest="$line"
    while [[ "$rest" =~ fromJSON\(([^\)]*)\) ]]; do
      arg="${BASH_REMATCH[1]}"
      before="${rest%%fromJSON($arg)*}"

      if [[ "$(squeeze "$before")" == *"$(squeeze "$arg")&&"* ]]; then
        : # guarded on its own operand
      else
        findings=$(( findings + 1 ))
        echo "${wf}:${lineno}: unguarded fromJSON (#375): write \`${arg} && fromJSON(${arg})...\` so an empty value short-circuits before fromJSON runs"
        echo "    ${trimmed}"
      fi

      rest="${rest#*fromJSON($arg)}"
    done
  done < "$wf"
done

if [ "$findings" -ne 0 ]; then
  echo
  echo "${findings} unguarded fromJSON call(s) across ${scanned} workflow file(s). A step's env: block is evaluated even when the step is SKIPPED, so an \`if:\` does not protect this — see #373/#374."
  exit 1
fi

echo "no unguarded fromJSON in ${scanned} workflow file(s)"
