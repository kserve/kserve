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

echo "Github SHA ${GITHUB_SHA}"
CONTROLLER_IMG_TAG=${DOCKER_REPO}/${CONTROLLER_IMG}:${GITHUB_SHA}
LOCALMODEL_CONTROLLER_IMG_TAG=${DOCKER_REPO}/${LOCALMODEL_CONTROLLER_IMG}:${GITHUB_SHA}
LOCALMODEL_AGENT_IMG_TAG=${DOCKER_REPO}/${LOCALMODEL_AGENT_IMG}:${GITHUB_SHA}
STORAGE_INIT_IMG_TAG=${DOCKER_REPO}/${STORAGE_INIT_IMG}:${GITHUB_SHA}
AGENT_IMG_TAG=${DOCKER_REPO}/${AGENT_IMG}:${GITHUB_SHA}
ROUTER_IMG_TAG=${DOCKER_REPO}/${ROUTER_IMG}:${GITHUB_SHA}

echo "Building Kserve controller image"
docker buildx build . -t "${CONTROLLER_IMG_TAG}" \
  -o type=docker,dest="${DOCKER_IMAGES_PATH}/${CONTROLLER_IMG}-${GITHUB_SHA}",compression-level=0

echo "Building localmodel controller image"
docker buildx build -f localmodel.Dockerfile . -t "${LOCALMODEL_CONTROLLER_IMG_TAG}" \
  -o type=docker,dest="${DOCKER_IMAGES_PATH}/${LOCALMODEL_CONTROLLER_IMG}-${GITHUB_SHA}",compression-level=0

echo "Building localmodel agent image"
docker buildx build -f localmodel-agent.Dockerfile . -t "${LOCALMODEL_AGENT_IMG_TAG}" \
  -o type=docker,dest="${DOCKER_IMAGES_PATH}/${LOCALMODEL_AGENT_IMG}-${GITHUB_SHA}",compression-level=0

echo "Building agent image"
docker buildx build -f agent.Dockerfile . -t "${AGENT_IMG_TAG}" \
  -o type=docker,dest="${DOCKER_IMAGES_PATH}/${AGENT_IMG}-${GITHUB_SHA}",compression-level=0

echo "Building router image"
docker buildx build -f router.Dockerfile . -t "${ROUTER_IMG_TAG}" \
  -o type=docker,dest="${DOCKER_IMAGES_PATH}/${ROUTER_IMG}-${GITHUB_SHA}",compression-level=0

echo "Disk usage before Building storage initializer:"
        df -hT

pushd python >/dev/null
  echo "Building storage initializer"
  docker buildx build -f storage-initializer.Dockerfile . -t "${STORAGE_INIT_IMG_TAG}" \
    -o type=docker,dest="${DOCKER_IMAGES_PATH}/${STORAGE_INIT_IMG}-${GITHUB_SHA}",compression-level=0
popd

echo "Done building images"
