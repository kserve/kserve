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
source "${SCRIPT_DIR}/image-cache.sh"

if [ -d "${DOCKER_IMAGES_PATH}" ]; then
  mkdir -p "${DOCKER_IMAGES_PATH}"  
fi

echo "Github SHA ${TAG}"
CONTROLLER_IMG_TAG=${KO_DOCKER_REPO}/${CONTROLLER_IMG}:${TAG}
LOCALMODEL_CONTROLLER_IMG_TAG=${KO_DOCKER_REPO}/${LOCALMODEL_CONTROLLER_IMG}:${TAG}
LOCALMODEL_AGENT_IMG_TAG=${KO_DOCKER_REPO}/${LOCALMODEL_AGENT_IMG}:${TAG}
KERNELCACHE_AGENT_IMG_TAG=${KO_DOCKER_REPO}/${KERNELCACHE_AGENT_IMG}:${TAG}
MCV_IMG_TAG=${KO_DOCKER_REPO}/${MCV_IMG}:${TAG}
STORAGE_INIT_IMG_TAG=${KO_DOCKER_REPO}/${STORAGE_INIT_IMG}:${TAG}
AGENT_IMG_TAG=${KO_DOCKER_REPO}/${AGENT_IMG}:${TAG}
ROUTER_IMG_TAG=${KO_DOCKER_REPO}/${ROUTER_IMG}:${TAG}
LLMISVC_CONTROLLER_IMG_TAG=${KO_DOCKER_REPO}/${LLMISVC_CONTROLLER_IMG}:${TAG}

types=("${1:-kserve}")


if [[ " ${types[*]} " =~ "llmisvc" ]]; then
  echo "Building LLMISvc controller image: ${LLMISVC_CONTROLLER_IMG_TAG}"
  build_image llmisvc-controller "${LLMISVC_CONTROLLER_IMG_TAG}" "${DOCKER_IMAGES_PATH}/${LLMISVC_CONTROLLER_IMG}-${TAG}"
  echo "Disk usage after Building LLMIsvc controller image:"
      df -hT
else
  echo "Building Kserve controller image"
  build_image kserve-controller "${CONTROLLER_IMG_TAG}" "${DOCKER_IMAGES_PATH}/${CONTROLLER_IMG}-${TAG}"

  echo "Building localmodel controller image"
  build_image kserve-localmodel-controller "${LOCALMODEL_CONTROLLER_IMG_TAG}" "${DOCKER_IMAGES_PATH}/${LOCALMODEL_CONTROLLER_IMG}-${TAG}"

  echo "Building localmodel agent image"
  build_image kserve-localmodelnode-agent "${LOCALMODEL_AGENT_IMG_TAG}" "${DOCKER_IMAGES_PATH}/${LOCALMODEL_AGENT_IMG}-${TAG}"

  echo "Building kernelcache agent image"
  build_image kserve-kernelcachenode-agent "${KERNELCACHE_AGENT_IMG_TAG}" "${DOCKER_IMAGES_PATH}/${KERNELCACHE_AGENT_IMG}-${TAG}"

  echo "Building mcv minimal image"
  build_image kserve-mcv-minimal "${MCV_IMG_TAG}-minimal" "${DOCKER_IMAGES_PATH}/${MCV_IMG}-${TAG}-minimal"

  echo "Building agent image"
  build_image agent "${AGENT_IMG_TAG}" "${DOCKER_IMAGES_PATH}/${AGENT_IMG}-${TAG}"

  echo "Building router image"
  build_image router "${ROUTER_IMG_TAG}" "${DOCKER_IMAGES_PATH}/${ROUTER_IMG}-${TAG}"

  echo "Disk usage before Building storage initializer:"
          df -hT
fi


echo "Building storage initializer"
build_image storage-initializer "${STORAGE_INIT_IMG_TAG}" "${DOCKER_IMAGES_PATH}/${STORAGE_INIT_IMG}-${TAG}"

echo "Done building images"
