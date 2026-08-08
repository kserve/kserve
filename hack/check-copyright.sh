#!/bin/bash

# Copyright 2026 The KServe Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Checks that all tracked Python and Go files with a KServe copyright header
# have a year no more than MAX_STALE_YEARS behind the current year.
# Exits with code 1 if violations are found.
#
# Run 'hack/boilerplate.sh' to fix stale years.

set -o nounset

CURRENT_YEAR=$(date +%Y)
MAX_STALE_YEARS=1
THRESHOLD_YEAR=$(( CURRENT_YEAR - MAX_STALE_YEARS ))
FAILED=0

check_file() {
  local file=$1

  # Skip files not tracked by git (untracked new files, etc.)
  if ! git ls-files --error-unmatch "$file" &>/dev/null 2>&1; then
    return
  fi

  # Only check files that carry a KServe-specific copyright header
  if ! grep -q "The KServe Authors" "$file"; then
    return
  fi

  local copyright_year
  copyright_year=$(grep -o "Copyright [0-9]\{4\} The KServe Authors" "$file" | grep -o "[0-9]\{4\}" | head -1)

  if [[ -z "$copyright_year" ]]; then
    echo "MISSING: $file (has 'The KServe Authors' but no parseable copyright year)"
    FAILED=1
    return
  fi

  if [[ "$copyright_year" -le "$THRESHOLD_YEAR" ]]; then
    echo "STALE: $file (copyright $copyright_year, threshold $THRESHOLD_YEAR)"
    FAILED=1
  fi
}

while IFS= read -r -d '' file; do
  check_file "$file"
done < <(find ./pkg ./cmd -name '*.go' -print0)

while IFS= read -r -d '' file; do
  check_file "$file"
done < <(find ./python -name '*.py' -print0)

if [[ "$FAILED" -eq 1 ]]; then
  echo ""
  echo "Copyright year check failed. Run 'hack/boilerplate.sh' from the project root to update stale headers."
  exit 1
fi

echo "Copyright year check passed."
