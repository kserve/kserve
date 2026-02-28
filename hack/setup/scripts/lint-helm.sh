#!/usr/bin/env bash

# lint-helm.sh
#
# Purpose: Run Helm lint on all charts to validate their structure and syntax.
#
# What it checks:
#   - Chart.yaml is well-formed
#   - Templates are valid YAML
#   - Required fields are present
#   - No syntax errors in Go templates
#
# Usage: bash hack/setup/scripts/lint-helm.sh
#
# Exit codes:
#   0 - All charts pass linting
#   1 - One or more charts failed linting

set -euo pipefail

echo "Linting Helm charts..."

exit_code=0
for chart in charts/*; do
  if [[ -f "$chart/Chart.yaml" ]]; then
    echo "→ Linting $(basename "$chart")..."
    if ! helm lint "$chart" --quiet 2>&1; then
      echo "✗ Linting failed for $(basename "$chart")"
      exit_code=1
    fi
  fi
done

if [ $exit_code -eq 0 ]; then
  echo "✓ All charts passed lint"
else
  echo ""
  echo "ERROR: One or more charts failed linting!"
  echo "Run 'helm lint charts/<chart-name>' for detailed error messages."
fi

exit $exit_code
