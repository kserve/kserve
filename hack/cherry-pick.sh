#
# Copyright 2022 The KServe Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

#!/bin/bash

set -e

if [[ $# == "0" ]]; then
  cat << EOF
  requirements:
  * Install gh, jq
  * Run this script from kubeflow/pipelines repo
  * You need permission to add labels to PRs
  usage: ./cherry-pick.sh <PR-number> [PR-numbers]
  for example: ./cherry-pick.sh 123 456 789
  You can get the list of PRs waiting to be cherrypicked by:
  1. Open https://github.com/kserve/kserve/pulls?q=is%3Apr+label%3Acherrypick-approved+-label%3Acherrypicked+is%3Amerged+sort%3Aupdated-asc+.
  2. Open browser console (usually by pressing F12).
  3. Paste the following command.
    console.log(Array.from(document.querySelectorAll('[id^="issue_"][id*="_link"]')).map(el => /issue_(.*)_link/.exec(el.id)[1]).join(' '))
EOF
fi

add_label_request_body=$(mktemp)
echo '{"labels":["cherrypicked"]}' > $add_label_request_body

for pr in "$@"
do
  echo "Cherry picking #$pr"
  LABELS_JSON=$(gh api repos/kserve/kserve/issues/$pr/labels)
  if echo "$LABELS_JSON" | grep cherrypick-approved >/dev/null; then
    echo "PR #$pr has cherrypick-approved label"
  else
    echo "ERROR: PR #$pr does not have cherrypick-approved label"
    exit 1
  fi
  if echo "$LABELS_JSON" | grep cherrypicked >/dev/null; then
    echo "SKIPPED: PR #$pr has already been cherry picked"
    continue
  fi
  MERGE_COMMIT_SHA=$(gh api repos/kserve/kserve/pulls/$pr  | jq -r .merge_commit_sha)
  echo "Merge commit SHA is $MERGE_COMMIT_SHA"
  git cherry-pick $MERGE_COMMIT_SHA
  # ref: https://docs.github.com/en/rest/reference/issues#add-labels-to-an-issue
  # pull request can also use issue api for adding labels
  echo "Adding cherrypicked label to PR $pr"
  gh api repos/kserve/kserve/issues/$pr/labels -X POST --input $add_label_request_body >/dev/null
done
