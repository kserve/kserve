#!/bin/bash
# Verify that kustomize patches in config/crd/full subdirectories are mirrored
# in config/crd/minimal.  Catches forgotten patch syncs before they reach CI.
set -eu -o pipefail

cd "$(dirname "$0")/.."

FULL_DIR="config/crd/full"
MINIMAL_DIR="config/crd/minimal"
rc=0

for full_subdir in "$FULL_DIR"/*/; do
  name=$(basename "$full_subdir")
  minimal_subdir="$MINIMAL_DIR/$name"

  full_kustomization="$full_subdir/kustomization.yaml"
  minimal_kustomization="$minimal_subdir/kustomization.yaml"

  # Skip subdirectories without a kustomization file.
  [[ -f "$full_kustomization" ]] || continue

  # --- Check kustomization.yaml exists in minimal ---
  if [[ ! -f "$minimal_kustomization" ]]; then
    echo "FAIL: $minimal_kustomization missing (exists in full)"
    rc=1
    continue
  fi

  # --- Compare patches section ---
  full_patches=$(yq '.patches // []' "$full_kustomization")
  minimal_patches=$(yq '.patches // []' "$minimal_kustomization")

  if [[ "$full_patches" != "$minimal_patches" ]]; then
    echo "FAIL: patches mismatch in $name/kustomization.yaml"
    echo "  full:    $full_patches"
    echo "  minimal: $minimal_patches"
    rc=1
  fi

  # --- Verify each referenced patch file exists and matches ---
  patch_paths=$(yq '.patches[].path' "$full_kustomization" 2>/dev/null || true)
  for patch in $patch_paths; do
    full_file="${full_subdir%/}/$patch"
    minimal_file="${minimal_subdir%/}/$patch"

    if [[ ! -f "$minimal_file" ]]; then
      echo "FAIL: patch file $minimal_file missing (exists at $full_file)"
      rc=1
      continue
    fi

    if ! diff -q "$full_file" "$minimal_file" > /dev/null 2>&1; then
      echo "FAIL: patch file differs: $full_file vs $minimal_file"
      diff "$full_file" "$minimal_file" || true
      rc=1
    fi
  done
done

if [[ $rc -eq 0 ]]; then
  echo "verify-minimal-crd-patches: OK"
fi

exit $rc
