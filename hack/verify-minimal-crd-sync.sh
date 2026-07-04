#!/bin/bash
# Verify that kustomize artifacts in config/crd/full subdirectories are
# mirrored in config/crd/minimal.  This covers kustomization.yaml (resources,
# patches, and any future sections) as well as non-CRD yaml files such as
# conversion webhook patches.
#
# CRD schema files (serving.kserve.io*.yaml) are intentionally different
# between full and minimal (minimal has validation stripped), so they are
# excluded from comparison.
set -eu -o pipefail

cd "$(dirname "$0")/.."

FULL_DIR="config/crd/full"
MINIMAL_DIR="config/crd/minimal"
rc=0

for full_subdir in "$FULL_DIR"/*/; do
  name=$(basename "$full_subdir")
  full_subdir="${full_subdir%/}"
  minimal_subdir="$MINIMAL_DIR/$name"

  # --- kustomization.yaml ---
  full_kustomization="$full_subdir/kustomization.yaml"
  [[ -f "$full_kustomization" ]] || continue

  minimal_kustomization="$minimal_subdir/kustomization.yaml"
  if [[ ! -f "$minimal_kustomization" ]]; then
    echo "FAIL: $minimal_kustomization missing (exists in full)"
    rc=1
    continue
  fi

  if ! diff -q "$full_kustomization" "$minimal_kustomization" > /dev/null 2>&1; then
    echo "FAIL: kustomization.yaml differs in $name/"
    diff -u "$full_kustomization" "$minimal_kustomization" || true
    rc=1
  fi

  # --- Non-CRD yaml files (patches, overlays, etc.) ---
  for file in "$full_subdir"/*.yaml; do
    [[ -f "$file" ]] || continue
    base=$(basename "$file")
    case "$base" in
      kustomization.yaml|serving.kserve.io*) continue ;;
    esac

    minimal_file="$minimal_subdir/$base"
    if [[ ! -f "$minimal_file" ]]; then
      echo "FAIL: $minimal_file missing (exists at $file)"
      rc=1
      continue
    fi

    if ! diff -q "$file" "$minimal_file" > /dev/null 2>&1; then
      echo "FAIL: $base differs in $name/"
      diff -u "$file" "$minimal_file" || true
      rc=1
    fi
  done
done

if [[ $rc -eq 0 ]]; then
  echo "verify-minimal-crd-sync: OK"
fi

exit $rc
