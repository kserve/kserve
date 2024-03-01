#!/bin/bash
set -eu -o pipefail

cd "$(dirname "$0")/.."

find config/crd/full -name 'serving.kserve.io*.yaml' | while read -r file; do
  echo "Patching ${file}"
  # create minimal
  minimal="config/crd/minimal/$(basename "$file")"
  echo "Creating ${minimal}"
  cp "$file" "$minimal"
  go run ./hack removecrdvalidation "$minimal"
done
