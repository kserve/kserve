#!/bin/bash

# Post (or update) a PR comment with the test-selector report.
#
# Usage:
#   post-test-selection-comment.sh <comment-marker> <title> <pr-number> \
#       <changed-files-path> <selector-json-path> <job1>=<val1> [<job2>=<val2> ...]
#
# Requires: gh (GitHub CLI), python3, GH_TOKEN env var.

set -o errexit
set -o nounset
set -o pipefail

COMMENT_MARKER="$1"; shift
TITLE="$1"; shift
PR_NUMBER="$1"; shift
CHANGED_FILES="$1"; shift
SELECTOR_JSON="$1"; shift

WILL_RUN=0
SKIPPED=0
TABLE="| Job | Status |"$'\n'"| --- | --- |"
for entry in "$@"; do
  job="${entry%%=*}"
  val="${entry#*=}"
  if [[ "$val" == "false" ]]; then
    TABLE+=$'\n'"| \`${job}\` | :next_track_button: skipped |"
    ((SKIPPED++)) || true
  else
    TABLE+=$'\n'"| \`${job}\` | :white_check_mark: will run |"
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

BODY="${COMMENT_MARKER}
### ${TITLE}

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

EXISTING=$(gh pr view "$PR_NUMBER" --json comments \
  --jq ".comments[] | select(.body | startswith(\"${COMMENT_MARKER}\")) | .id" \
  | tail -1)

if [[ -n "$EXISTING" ]]; then
  gh api graphql \
    -f query='mutation($id:ID!,$body:String!){updateIssueComment(input:{id:$id,body:$body}){issueComment{id}}}' \
    -f id="$EXISTING" -f body="$BODY"
else
  gh pr comment "$PR_NUMBER" --body "$BODY"
fi
