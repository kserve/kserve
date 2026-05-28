#!/bin/bash
set -eu -o pipefail

cd "$(dirname "$0")/.."

FULL_DIR="config/crd/full"
MINIMAL_DIR="config/crd/minimal"

# Copy CRD files from a full directory to the corresponding minimal directory,
# stripping OpenAPI validation from each file.
strip_and_copy_crds() {
  local src_dir="$1"
  local dst_dir="$2"

  find "$src_dir" -maxdepth 1 -name 'serving.kserve.io*.yaml' | while read -r file; do
    minimal="$dst_dir/$(basename "$file")"
    echo "Creating minimal CRD file: ${minimal}"
    cp "$file" "$minimal"
    go run ./cmd/crd-gen removecrdvalidation "$minimal"
  done
}

# Sync kustomize artifacts (kustomization.yaml and patch files) from a full
# subdirectory to the corresponding minimal subdirectory.  CRD schema files
# (serving.kserve.io*.yaml) are handled separately by strip_and_copy_crds.
sync_kustomize_patches() {
  local src_dir="$1"
  local dst_dir="$2"

  # Sync kustomization.yaml so patch references stay in lockstep.
  if [[ -f "$src_dir/kustomization.yaml" ]]; then
    cp "$src_dir/kustomization.yaml" "$dst_dir/kustomization.yaml"
  fi

  # Copy any non-CRD yaml files (e.g. conversion webhook patches).
  for file in "$src_dir"/*.yaml; do
    [[ -f "$file" ]] || continue
    local base
    base=$(basename "$file")
    case "$base" in
      kustomization.yaml|serving.kserve.io*) continue ;;
    esac
    echo "Syncing patch file: $dst_dir/$base"
    cp "$file" "$dst_dir/$base"
  done
}

# --- Root-level CRDs (no subdirectory) ---
strip_and_copy_crds "$FULL_DIR" "$MINIMAL_DIR"

# --- Subdirectory CRDs + kustomize patches ---
for subdir in "$FULL_DIR"/*/; do
  name=$(basename "$subdir")
  strip_and_copy_crds "$subdir" "$MINIMAL_DIR/$name"
  sync_kustomize_patches "$subdir" "$MINIMAL_DIR/$name"
done
