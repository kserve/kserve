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

# The script is used to build all the queue-proxy extension image.

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
export QPEXT_IMG=${KO_DOCKER_REPO}/${QPEXT_IMG}:${TAG}


echo "Building queue proxy extension image"
docker buildx build -t ${QPEXT_IMG} -f qpext/qpext.Dockerfile .
echo "Done building image"
