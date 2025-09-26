#!/bin/bash

# Copyright 2024 The KServe Authors.
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

# Minimal build script for LLM E2E tests - only builds required images

set -o errexit
set -o nounset
set -o pipefail

echo "Github SHA ${GITHUB_SHA}"

# Only build the 2 images needed for LLM tests
LLMISVC_CONTROLLER_IMG_TAG=${DOCKER_REPO}/${LLMISVC_CONTROLLER_IMG}:${GITHUB_SHA}
STORAGE_INIT_IMG_TAG=${DOCKER_REPO}/${STORAGE_INIT_IMG}:${GITHUB_SHA}

echo "Building LLM controller image"
docker buildx build -f llmisvc-controller.Dockerfile . -t "${LLMISVC_CONTROLLER_IMG_TAG}" \
  -o type=docker,dest="${DOCKER_IMAGES_PATH}/${LLMISVC_CONTROLLER_IMG}-${GITHUB_SHA}",compression-level=0

echo "Building storage initializer image"
docker buildx build -f storage-initializer.Dockerfile . -t "${STORAGE_INIT_IMG_TAG}" \
  -o type=docker,dest="${DOCKER_IMAGES_PATH}/${STORAGE_INIT_IMG}-${GITHUB_SHA}",compression-level=0

echo "✅ LLM images built successfully!"
echo "Built images:"
echo "  - ${LLMISVC_CONTROLLER_IMG_TAG}"
echo "  - ${STORAGE_INIT_IMG_TAG}"
