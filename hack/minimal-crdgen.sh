#!/bin/bash
set -eu -o pipefail

cd "$(dirname "$0")/.."

find config/crd/full -maxdepth 1 -name 'serving.kserve.io*.yaml' | while read -r file; do
  # create minimal
  minimal="config/crd/minimal/$(basename "$file")"
  echo "Creating minimal CRD file: ${minimal}"
  cp "$file" "$minimal"
  go run ./cmd/crd-gen removecrdvalidation "$minimal"
done

find config/crd/full/llmisvc -name 'serving.kserve.io*.yaml' | while read -r file; do
  # create minimal
  minimal="config/crd/minimal/llmisvc/$(basename "$file")"
  echo "Creating minimal CRD file: ${minimal}"
  cp "$file" "$minimal"
  go run ./cmd/crd-gen removecrdvalidation "$minimal"
done

find config/crd/full/localmodel -name 'serving.kserve.io*.yaml' | while read -r file; do
  # create minimal
  minimal="config/crd/minimal/localmodel/$(basename "$file")"
  echo "Creating minimal CRD file: ${minimal}"
  cp "$file" "$minimal"
  go run ./cmd/crd-gen removecrdvalidation "$minimal"
done
