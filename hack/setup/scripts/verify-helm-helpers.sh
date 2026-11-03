#!/usr/bin/env bash

# verify-helm-helpers.sh
#
# Purpose: Verify that shared helper functions (deepMerge, replaceNamespace) are identical
#          across all KServe Helm resource charts.
#
# Background: These shared helpers are duplicated in each chart (instead of using a library chart)
#             to avoid the complexity of Helm dependencies. This script ensures they stay
#             in sync to prevent subtle bugs from helper function drift.
#
# Note: Each chart may have additional chart-specific helpers, which are not compared.
#
# Usage: bash hack/setup/scripts/verify-helm-helpers.sh
#
# Exit codes:
#   0 - All shared helper functions are consistent
#   1 - Shared helper functions differ between charts

set -euo pipefail

CHARTS=(
  "kserve-resources"
  "kserve-llmisvc-resources"
  "kserve-localmodel-resources"
  "kserve-runtime-configs"
)

# Only check these shared helper functions
HELPERS=("deepMerge" "mergeArrayByName" "replaceNamespace")

echo "Verifying Helm helper function consistency..."

TEMP_DIR=$(mktemp -d)
trap 'rm -rf "$TEMP_DIR"' EXIT

# Extract each shared helper function from charts and normalize chart names
# For each chart: charts/*/templates/_helpers.tpl
for chart in "${CHARTS[@]}"; do
  # For each helper function: deepMerge, replaceNamespace
  for helper in "${HELPERS[@]}"; do
    # Extract helper function definition and normalize chart name to CHARTNAME
    sed -n "/define.*${helper}/,/^{{- end }}/p" \
      "charts/${chart}/templates/_helpers.tpl" | \
      sed 's/kserve-resources/CHARTNAME/g; s/llm-isvc-resources/CHARTNAME/g; s/kserve-localmodel-resources/CHARTNAME/g; s/kserve-runtime-configs/CHARTNAME/g' \
      > "${TEMP_DIR}/${chart}-${helper}.txt"
  done
done

# Compare each helper function across all charts
exit_code=0
# For each helper function: deepMerge, replaceNamespace
for helper in "${HELPERS[@]}"; do
  echo "→ Checking ${helper} consistency..."
  base="${TEMP_DIR}/${CHARTS[0]}-${helper}.txt"

  # Compare kserve-resources (base) with other charts
  for chart in "${CHARTS[@]:1}"; do
    if ! diff -q "$base" "${TEMP_DIR}/${chart}-${helper}.txt" > /dev/null 2>&1; then
      echo "✗ ${helper} differs between ${CHARTS[0]} and ${chart}"
      echo "Showing differences:"
      diff "$base" "${TEMP_DIR}/${chart}-${helper}.txt" || true
      exit_code=1
    fi
  done
done

if [ $exit_code -eq 0 ]; then
  echo "✓ All helper functions are consistent across charts"
else
  echo ""
  echo "ERROR: Helper function inconsistencies detected!"
  echo "Please ensure all charts use identical ${HELPERS[*]} helper functions."
fi

exit $exit_code
