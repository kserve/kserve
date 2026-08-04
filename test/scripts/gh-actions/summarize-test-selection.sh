#!/bin/bash

# Render the test-selector report as a GitHub Actions job summary.
#
# Writes markdown to $GITHUB_STEP_SUMMARY (falling back to stdout when unset),
# so it works for fork PRs without a write-scoped token.
#
# Usage:
#   summarize-test-selection.sh <title> \
#       <changed-files-path> <selector-json-path> <job1>=<val1> [<job2>=<val2> ...]
#
# Requires: python3.

set -o errexit
set -o nounset
set -o pipefail

TITLE="$1"; shift
CHANGED_FILES="$1"; shift
SELECTOR_JSON="$1"; shift

WILL_RUN=0
SKIPPED=0
WILL_RUN_JOBS=()
TABLE="| Job | Status |"$'\n'"| --- | --- |"
for entry in "$@"; do
  job="${entry%%=*}"
  val="${entry#*=}"
  if [[ "$val" == "false" ]]; then
    TABLE+=$'\n'"| \`${job}\` | :next_track_button: skipped |"
    ((SKIPPED++)) || true
  else
    TABLE+=$'\n'"| \`${job}\` | :white_check_mark: will run |"
    WILL_RUN_JOBS+=("$job")
    ((WILL_RUN++)) || true
  fi
done
TOTAL=$((WILL_RUN + SKIPPED))

MARKERS=$(python3 -c "
import json, sys
d = json.load(open('$SELECTOR_JSON'))
markers = d.get('e2e_tests', {}).get('markers', [])
print(', '.join(f'\`{m}\`' for m in markers) if markers else '_none_')
" 2>/dev/null || echo "_unavailable_")

REASONS=$(python3 -c "
import json
d = json.load(open('$SELECTOR_JSON'))
for r in d.get('reasons', []):
    print(f'- \`{r}\`')
" 2>/dev/null || echo "_unavailable_")

FILE_COUNT=$(wc -l < "$CHANGED_FILES" | tr -d ' ')
CHANGED_LIST=$(sed 's/^/- `/' "$CHANGED_FILES" | sed 's/$/`/')

RAW_JSON=$(python3 -c "
import json
print(json.dumps(json.load(open('$SELECTOR_JSON')), indent=2))
" 2>/dev/null || echo "{}")

BODY="### ${TITLE}

**${WILL_RUN}/${TOTAL}** jobs selected (${SKIPPED} skipped)

${TABLE}

<details>
<summary>Selected markers</summary>

${MARKERS}

</details>

<details>
<summary>Changed files (${FILE_COUNT})</summary>

${CHANGED_LIST}

</details>

<details>
<summary>Selection reasons</summary>

${REASONS}

</details>

<details>
<summary>Raw selector output</summary>

\`\`\`json
${RAW_JSON}
\`\`\`

</details>"

printf '%s\n' "$BODY" >> "${GITHUB_STEP_SUMMARY:-/dev/stdout}"

# Surface a one-line headline as a workflow annotation (fork-safe, no token
# needed). Annotations appear on the run/checks page without drilling into the
# job, and must be a single line (newlines break the annotation command).
RUN_URL="${GITHUB_SERVER_URL:-https://github.com}/${GITHUB_REPOSITORY:-}/actions/runs/${GITHUB_RUN_ID:-}"
if ((WILL_RUN > 0)); then
  RUN_LIST=$(IFS=, ; echo "${WILL_RUN_JOBS[*]}")
  NOTICE="${WILL_RUN}/${TOTAL} jobs selected (${SKIPPED} skipped): ${RUN_LIST}. See job summary: ${RUN_URL}"
else
  NOTICE="0/${TOTAL} jobs selected (all skipped). See job summary: ${RUN_URL}"
fi
echo "::notice title=${TITLE}::${NOTICE}"

# Echo the full selector detail to the step log (collapsed group) so it is
# inspectable without downloading the artifact.
echo "::group::${TITLE} - selector detail"
printf '%s\n' "$RAW_JSON"
echo "::endgroup::"
