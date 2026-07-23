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

# Build KServe runtime server images.
#
# Usage:
#   build-server-runtimes.sh predictor,transformer   # build all predictor + transformer images (legacy)
#   build-server-runtimes.sh sklearn                  # build a single image by name
#   build-server-runtimes.sh                          # defaults to "predictor"

set -o errexit
set -o nounset
set -o pipefail

# Load image configurations
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd)"
source "${PROJECT_ROOT}/kserve-images.sh"

AUTOGLUON_IMG="${AUTOGLUON_IMG:-autogluonserver}"
KO_DOCKER_REPO="${KO_DOCKER_REPO:-kserve}"

# Image registry: name -> dockerfile (relative to python/)
declare -A IMAGE_DOCKERFILE=(
  [sklearn]="sklearn.Dockerfile"
  [xgb]="xgb.Dockerfile"
  [lgb]="lgb.Dockerfile"
  [pmml]="pmml.Dockerfile"
  [paddle]="paddle.Dockerfile"
  [autogluon]="autogluon.Dockerfile"
  [custom-model-grpc]="custom_model_grpc.Dockerfile"
  [custom-transformer-grpc]="custom_transformer_grpc.Dockerfile"
  [huggingface]="huggingface_server_cpu.Dockerfile"
  [predictive]="predictiveserver.Dockerfile"
  [art]="artexplainer.Dockerfile"
  [image-transformer]="custom_transformer.Dockerfile"
)

# Image registry: name -> env var holding the image basename
declare -A IMAGE_ENVVAR=(
  [sklearn]="SKLEARN_IMG"
  [xgb]="XGB_IMG"
  [lgb]="LGB_IMG"
  [pmml]="PMML_IMG"
  [paddle]="PADDLE_IMG"
  [autogluon]="AUTOGLUON_IMG"
  [custom-model-grpc]="CUSTOM_MODEL_GRPC_IMG"
  [custom-transformer-grpc]="CUSTOM_TRANSFORMER_GRPC_IMG"
  [huggingface]="HUGGINGFACE_IMG"
  [predictive]="PREDICTIVE_IMG"
  [art]="ART_IMG"
  [image-transformer]="IMAGE_TRANSFORMER_IMG"
)

# Artifact prefix per image (matches workflow upload naming convention)
declare -A IMAGE_PREFIX=(
  [sklearn]="pred" [xgb]="pred" [lgb]="pred" [pmml]="pred"
  [paddle]="pred" [autogluon]="pred" [custom-model-grpc]="pred"
  [predictive]="pred" [huggingface]=""
  [custom-transformer-grpc]="trans" [image-transformer]="trans"
  [art]="exp"
)

# Group definitions for backward compatibility
PREDICTOR_IMAGES=(sklearn xgb lgb pmml paddle autogluon custom-model-grpc custom-transformer-grpc huggingface predictive)
EXPLAINER_IMAGES=(art)
TRANSFORMER_IMAGES=(image-transformer)

if [[ "${1:-}" == "--list-json" ]]; then
  IFS=, read -ra groups <<< "${2:-predictor}"
  names=()
  for group in "${groups[@]}"; do
    case "$group" in
      predictor)    names+=("${PREDICTOR_IMAGES[@]}") ;;
      explainer)    names+=("${EXPLAINER_IMAGES[@]}") ;;
      transformer)  names+=("${TRANSFORMER_IMAGES[@]}") ;;
      *)            echo "Unknown group: $group" >&2; exit 1 ;;
    esac
  done
  json='{"image":['
  first=true
  for name in "${names[@]}"; do
    prefix="${IMAGE_PREFIX[$name]:-}"
    envvar="${IMAGE_ENVVAR[$name]}"
    $first || json+=','
    json+="{\"name\":\"${name}\",\"artifact_prefix\":\"${prefix}\",\"image_env\":\"${envvar}\"}"
    first=false
  done
  json+=']}'
  echo "$json"
  exit 0
fi

build_one() {
  local name="$1"
  local dockerfile="${IMAGE_DOCKERFILE[$name]:-}"
  local envvar="${IMAGE_ENVVAR[$name]:-}"

  if [[ -z "$dockerfile" || -z "$envvar" ]]; then
    echo "ERROR: Unknown image name: $name"
    echo "Available images: ${!IMAGE_DOCKERFILE[*]}"
    exit 1
  fi

  local img_basename="${!envvar}"
  local img_tag="${KO_DOCKER_REPO}/${img_basename}:${TAG}"
  local output="${DOCKER_IMAGES_PATH}/${img_basename}-${TAG}"

  echo "Building ${name} image (${dockerfile} -> ${img_basename})"
  docker buildx build -t "${img_tag}" -f "${dockerfile}" \
    -o "type=docker,dest=${output},compression-level=0" .
  echo "Disk usage after building ${name}:"
  df -hT
}

mkdir -p "${DOCKER_IMAGES_PATH}"
echo "Github SHA ${TAG}"

# Parse arguments
IFS=, read -ra targets <<< "${1:-predictor}"

pushd python >/dev/null

for target in "${targets[@]}"; do
  case "$target" in
    predictor)
      for img in "${PREDICTOR_IMAGES[@]}"; do build_one "$img"; done
      ;;
    explainer)
      for img in "${EXPLAINER_IMAGES[@]}"; do build_one "$img"; done
      ;;
    transformer)
      for img in "${TRANSFORMER_IMAGES[@]}"; do build_one "$img"; done
      ;;
    *)
      build_one "$target"
      ;;
  esac
done

popd >/dev/null

echo "Done building images"
