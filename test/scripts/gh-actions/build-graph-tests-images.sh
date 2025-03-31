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

echo "Github SHA ${GITHUB_SHA}"
export DOCKER_REPO=kserve
export SUCCESS_200_ISVC_IMG=success-200-isvc
export ERROR_404_ISVC_IMG=error-404-isvc
export DOCKER_IMAGES_PATH=/tmp/docker-images
: "${QUAY_REPO:=}"
SUCCESS_200_ISVC_IMG_TAG=${QUAY_REPO}/${DOCKER_REPO}/${SUCCESS_200_ISVC_IMG}:${GITHUB_SHA}
ERROR_404_ISVC_IMG_TAG=${QUAY_REPO}/${DOCKER_REPO}/${ERROR_404_ISVC_IMG}:${GITHUB_SHA}
if [ -z "${QUAY_REPO}" ]; then
  SUCCESS_200_ISVC_IMG_TAG=${DOCKER_REPO}/${SUCCESS_200_ISVC_IMG}:${GITHUB_SHA}
  ERROR_404_ISVC_IMG_TAG=${DOCKER_REPO}/${ERROR_404_ISVC_IMG}:${GITHUB_SHA}
fi


: "${BUILDER:=docker}"
if [ $BUILDER == "docker" ]; then
  BUILDER=docker
  BUILDER_TYPE=docker
  # docker buildx create --name mybuilder --driver docker-container --use
  else 
  BUILDER=podman
  BUILDER_TYPE=local
fi

pushd python >/dev/null
echo "Building success_200_isvc image"
$BUILDER buildx build -t "${SUCCESS_200_ISVC_IMG_TAG}" -f success_200_isvc.Dockerfile \
  -o type=${BUILDER_TYPE} .
echo "Done building success_200_isvc image"
echo "Building error_404_isvc image"
$BUILDER buildx build -t "${ERROR_404_ISVC_IMG_TAG}" -f error_404_isvc.Dockerfile \
  -o type=${BUILDER_TYPE} .
echo "Done building error_404_isvc image"
if $RUNNING_LOCAL; then
  $BUILDER push ${SUCCESS_200_ISVC_IMG_TAG}
  $BUILDER push ${ERROR_404_ISVC_IMG_TAG}
fi 
popd
echo "Done building images"

