#!/usr/bin/env bash
#
# Assert no workflow or repository script pipes a writer into a consumer that
# exits early (#402).
#
# `grep -q` and `head -n` stop reading the moment they have their answer. That
# closes the pipe while the writer is still writing, the writer dies on EPIPE,
# and `set -o pipefail` adopts the writer's status as the pipeline's — so
# `if ! producer | grep -q PATTERN` takes its FAILURE branch precisely when the
# pattern IS found. A guard built that way reports a violation that is not there,
# names a real and serious property while doing so, and sends the reader off to
# restore something that was never missing.
#
# ⚠️ Why a text check rather than "just remember": the defect is invisible until
# the writer's output outgrows the pipe buffer, and then it is a RACE rather than
# a clean threshold — yaadegar's publish gate passed on a push run and failed on a
# pull-request run minutes apart, on byte-identical files. Nothing about reading
# the line tells you which side of that you are on, and a test of the guard's
# behaviour passes whenever the race happens to go the right way. The shape is
# the only part that is deterministic, so the shape is what is asserted.
#
# ✅ The fix is a here-string: `grep -q PATTERN <<<"$value"`. There is no second
# process to race with, and it is shorter than what it replaces.
#
# A pass here means "no pipeline in these files ends in an early-exiting reader",
# and nothing wider. `| awk`, `| sed`, `| wc` and friends read to EOF and are not
# flagged; a genuine need for `| head` in a pipeline whose writer cannot be
# converted would have to be argued in review rather than silently allowed.
set -euo pipefail

self="$(basename "${BASH_SOURCE[0]}")"
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$root"

# Files to inspect. This script and its own tests are excluded: they necessarily
# contain the offending text as data (patterns, fixtures, documentation), and a
# check that flagged its own description could never pass.
mapfile -t files < <(
  {
    find .github/workflows -type f \( -name '*.yml' -o -name '*.yaml' \)
    find .github/scripts -type f -name '*.sh'
  } | grep -v "/${self}$" | grep -v "/${self%.sh}.test.sh$" | sort
)

if [ "${#files[@]}" -eq 0 ]; then
  echo "no workflow or script files found to check (#402): this guard is inspecting nothing"
  exit 1
fi

# `| grep …q…` or `| head`, allowing for flag clusters (-q, -qi, -Eq, --quiet)
# and for the pipe being written with or without surrounding spaces.
pattern='\|[[:space:]]*(grep[[:space:]]+(-[A-Za-z]*q[A-Za-z]*|--quiet)|head([[:space:]]|$))'

found=0
for f in "${files[@]}"; do
  while IFS=: read -r lineno text; do
    [ -n "${lineno:-}" ] || continue
    # Skip comment-only lines. A comment cannot execute, and the rule has to be
    # describable in prose somewhere — including in the files it inspects, which
    # is where the explanation is most use to whoever trips it. A trailing comment
    # on a line that also carries code is still scanned, so this cannot be used to
    # smuggle the shape past the check.
    case "$(printf '%s' "$text" | sed 's/^[[:space:]]*//')" in
      '#'*) continue ;;
    esac
    printf '%s:%s: %s\n' "$f" "$lineno" "$(printf '%s' "$text" | sed 's/^[[:space:]]*//')"
    found=1
  done < <(grep -nE "$pattern" "$f" || true)
done

if [ "$found" -ne 0 ]; then
  cat <<'MSG'

Each line above pipes into a reader that exits early, which makes the pipeline's
status depend on a race with the writer under `set -o pipefail` (#402). Rewrite as
a here-string:

    if ! printf '%s\n' "$value" | grep -q PATTERN; then   # racy
    if ! grep -q PATTERN <<<"$value"; then                # deterministic
MSG
  exit 1
fi

echo "no pipeline in ${#files[@]} workflow/script files feeds an early-exiting reader"
