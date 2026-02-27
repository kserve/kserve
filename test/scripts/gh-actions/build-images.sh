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

# The script is used to build all the KServe images.

# TODO: Implement selective building and tag replacement based on modified code.

set -o errexit
set -o nounset
set -o pipefail

# Load image configurations
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd)"
source "${PROJECT_ROOT}/kserve-images.sh"

if [ -d "${DOCKER_IMAGES_PATH}" ]; then
  mkdir -p "${DOCKER_IMAGES_PATH}"  
fi

echo "Github SHA ${TAG}"
CONTROLLER_IMG_TAG=${KO_DOCKER_REPO}/${CONTROLLER_IMG}:${TAG}
LOCALMODEL_CONTROLLER_IMG_TAG=${KO_DOCKER_REPO}/${LOCALMODEL_CONTROLLER_IMG}:${TAG}
LOCALMODEL_AGENT_IMG_TAG=${KO_DOCKER_REPO}/${LOCALMODEL_AGENT_IMG}:${TAG}
STORAGE_INIT_IMG_TAG=${KO_DOCKER_REPO}/${STORAGE_INIT_IMG}:${TAG}
AGENT_IMG_TAG=${KO_DOCKER_REPO}/${AGENT_IMG}:${TAG}
ROUTER_IMG_TAG=${KO_DOCKER_REPO}/${ROUTER_IMG}:${TAG}
LLMISVC_CONTROLLER_IMG_TAG=${KO_DOCKER_REPO}/${LLMISVC_CONTROLLER_IMG}:${TAG}

types=("${1:-kserve}")


if [[ " ${types[*]} " =~ "llmisvc" ]]; then
  echo "Building LLMISvc controller image: ${LLMISVC_CONTROLLER_IMG_TAG}"
  docker buildx build -f llmisvc-controller.Dockerfile . -t "${LLMISVC_CONTROLLER_IMG_TAG}" \
    -o type=docker,dest="${DOCKER_IMAGES_PATH}/${LLMISVC_CONTROLLER_IMG}-${TAG}",compression-level=0
  echo "Disk usage after Building LLMIsvc controller image:"
      df -hT
else
  echo "Building Kserve controller image"
  docker buildx build . -t "${CONTROLLER_IMG_TAG}" \
    -o type=docker,dest="${DOCKER_IMAGES_PATH}/${CONTROLLER_IMG}-${TAG}",compression-level=0

  echo "Building localmodel controller image"
  docker buildx build -f localmodel.Dockerfile . -t "${LOCALMODEL_CONTROLLER_IMG_TAG}" \
    -o type=docker,dest="${DOCKER_IMAGES_PATH}/${LOCALMODEL_CONTROLLER_IMG}-${TAG}",compression-level=0

  echo "Building localmodel agent image"
  docker buildx build -f localmodel-agent.Dockerfile . -t "${LOCALMODEL_AGENT_IMG_TAG}" \
    -o type=docker,dest="${DOCKER_IMAGES_PATH}/${LOCALMODEL_AGENT_IMG}-${TAG}",compression-level=0

  echo "Building agent image"
  docker buildx build -f agent.Dockerfile . -t "${AGENT_IMG_TAG}" \
    -o type=docker,dest="${DOCKER_IMAGES_PATH}/${AGENT_IMG}-${TAG}",compression-level=0

  echo "Building router image"
  docker buildx build -f router.Dockerfile . -t "${ROUTER_IMG_TAG}" \
    -o type=docker,dest="${DOCKER_IMAGES_PATH}/${ROUTER_IMG}-${TAG}",compression-level=0

  echo "Disk usage before Building storage initializer:"
          df -hT
fi


pushd python >/dev/null
  echo "Building storage initializer"
  docker buildx build -f storage-initializer.Dockerfile . -t "${STORAGE_INIT_IMG_TAG}" \
    -o type=docker,dest="${DOCKER_IMAGES_PATH}/${STORAGE_INIT_IMG}-${TAG}",compression-level=0
popd

echo "Done building images"
