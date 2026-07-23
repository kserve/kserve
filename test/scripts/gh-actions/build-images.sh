#!/bin/bash

# Copyright 2022 The KServe Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Build KServe core images.
#
# Usage:
#   build-images.sh              # build all kserve images (default)
#   build-images.sh kserve       # build all kserve images (explicit)
#   build-images.sh llmisvc      # build llmisvc controller only
#   build-images.sh controller   # build a single image by name

set -o errexit
set -o nounset
set -o pipefail

# Load image configurations
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd)"
source "${PROJECT_ROOT}/kserve-images.sh"

KO_DOCKER_REPO="${KO_DOCKER_REPO:-kserve}"

# Image registry: name -> "dockerfile|context_dir|env_var"
# context_dir is relative to PROJECT_ROOT ("." for repo root, "python" for python/)
declare -A IMAGE_REGISTRY=(
  [controller]="Dockerfile|.|CONTROLLER_IMG"
  [localmodel-controller]="localmodel.Dockerfile|.|LOCALMODEL_CONTROLLER_IMG"
  [localmodel-agent]="localmodel-agent.Dockerfile|.|LOCALMODEL_AGENT_IMG"
  [agent]="agent.Dockerfile|.|AGENT_IMG"
  [router]="router.Dockerfile|.|ROUTER_IMG"
  [storage-initializer]="storage-initializer.Dockerfile|python|STORAGE_INIT_IMG"
  [llmisvc-controller]="llmisvc-controller.Dockerfile|.|LLMISVC_CONTROLLER_IMG"
)

# Group definitions
KSERVE_IMAGES=(controller localmodel-controller localmodel-agent agent router storage-initializer)
LLMISVC_IMAGES=(llmisvc-controller)

build_one() {
  local name="$1"
  local entry="${IMAGE_REGISTRY[$name]:-}"

  if [[ -z "$entry" ]]; then
    echo "ERROR: Unknown image name: $name"
    echo "Available images: ${!IMAGE_REGISTRY[*]}"
    exit 1
  fi

  IFS='|' read -r dockerfile context_dir envvar <<< "$entry"
  local img_basename="${!envvar}"
  local img_tag="${KO_DOCKER_REPO}/${img_basename}:${TAG}"
  local output="${DOCKER_IMAGES_PATH}/${img_basename}-${TAG}"
  local build_dir="${PROJECT_ROOT}/${context_dir}"

  echo "Building ${name} image (${dockerfile} in ${context_dir}/ -> ${img_basename})"
  docker buildx build -f "${build_dir}/${dockerfile}" "${build_dir}" -t "${img_tag}" \
    -o "type=docker,dest=${output},compression-level=0"
  echo "Disk usage after building ${name}:"
  df -hT
}

if [[ "${1:-}" == "--list-json" ]]; then
  group="${2:-kserve}"
  case "$group" in
    kserve)   names=("${KSERVE_IMAGES[@]}") ;;
    llmisvc)  names=("${LLMISVC_IMAGES[@]}") ;;
    *)        echo "Unknown group: $group" >&2; exit 1 ;;
  esac
  json='{"image":['
  first=true
  for name in "${names[@]}"; do
    IFS='|' read -r _ _ envvar <<< "${IMAGE_REGISTRY[$name]}"
    $first || json+=','
    json+="{\"name\":\"${name}\",\"image_env\":\"${envvar}\"}"
    first=false
  done
  json+=']}'
  echo "$json"
  exit 0
fi

mkdir -p "${DOCKER_IMAGES_PATH}"
echo "Github SHA ${TAG}"

target="${1:-kserve}"

case "$target" in
  kserve)
    for img in "${KSERVE_IMAGES[@]}"; do build_one "$img"; done
    ;;
  llmisvc)
    for img in "${LLMISVC_IMAGES[@]}"; do build_one "$img"; done
    ;;
  *)
    build_one "$target"
    ;;
esac

echo "Done building images"
