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
IFS=,

echo "Github SHA ${GITHUB_SHA}"
# Predictor runtime server images
SKLEARN_IMG_TAG=${DOCKER_REPO}/${SKLEARN_IMG}:${GITHUB_SHA}
XGB_IMG_TAG=${DOCKER_REPO}/${XGB_IMG}:${GITHUB_SHA}
LGB_IMG_TAG=${DOCKER_REPO}/${LGB_IMG}:${GITHUB_SHA}
PMML_IMG_TAG=${DOCKER_REPO}/${PMML_IMG}:${GITHUB_SHA}
PADDLE_IMG_TAG=${DOCKER_REPO}/${PADDLE_IMG}:${GITHUB_SHA}
CUSTOM_MODEL_GRPC_IMG_TAG=${DOCKER_REPO}/${CUSTOM_MODEL_GRPC_IMG}:${GITHUB_SHA}
CUSTOM_TRANSFORMER_GRPC_IMG_TAG=${DOCKER_REPO}/${CUSTOM_TRANSFORMER_GRPC_IMG}:${GITHUB_SHA}
HUGGINGFACE_CPU_IMG_TAG=${DOCKER_REPO}/${HUGGINGFACE_IMG}:${GITHUB_SHA}
# Explainer images
ART_IMG_TAG=${DOCKER_REPO}/${ART_IMG}:${GITHUB_SHA}
# Transformer images
IMAGE_TRANSFORMER_IMG_TAG=${DOCKER_REPO}/${IMAGE_TRANSFORMER_IMG}:${GITHUB_SHA}
types=("$1")

pushd python >/dev/null
  if [[ " ${types[*]} " =~ "predictor" ]]; then
    echo "Building Sklearn image"
    docker buildx build -t "${SKLEARN_IMG_TAG}" -f sklearn.Dockerfile \
      -o type=docker,dest="${DOCKER_IMAGES_PATH}/${SKLEARN_IMG}-${GITHUB_SHA}",compression-level=0 .
    echo "Disk usage after Building Sklearn image:"
        df -hT
    echo "Building XGB image"
    docker buildx build -t "${XGB_IMG_TAG}" -f xgb.Dockerfile \
      -o type=docker,dest="${DOCKER_IMAGES_PATH}/${XGB_IMG}-${GITHUB_SHA}",compression-level=0 .
    echo "Disk usage after Building XGB image:"
        df -hT
    echo "Building LGB image"
    docker buildx build -t "${LGB_IMG_TAG}" -f lgb.Dockerfile \
      -o type=docker,dest="${DOCKER_IMAGES_PATH}/${LGB_IMG}-${GITHUB_SHA}",compression-level=0 .
    echo "Disk usage after Building LGB image:"
        df -hT
    echo "Building PMML image"
    docker buildx build -t "${PMML_IMG_TAG}" -f pmml.Dockerfile \
      -o type=docker,dest="${DOCKER_IMAGES_PATH}/${PMML_IMG}-${GITHUB_SHA}",compression-level=0 .
    echo "Disk usage after Building PMML image:"
        df -hT
    echo "Building Paddle image"
    docker buildx build -t "${PADDLE_IMG_TAG}" -f paddle.Dockerfile \
      -o type=docker,dest="${DOCKER_IMAGES_PATH}/${PADDLE_IMG}-${GITHUB_SHA}",compression-level=0 .
    echo "Disk usage after Building Paddle image:"
        df -hT
    echo "Building Custom model gRPC image"
    docker buildx build -t "${CUSTOM_MODEL_GRPC_IMG_TAG}" -f custom_model_grpc.Dockerfile \
      -o type=docker,dest="${DOCKER_IMAGES_PATH}/${CUSTOM_MODEL_GRPC_IMG}-${GITHUB_SHA}",compression-level=0 .
    echo "Disk usage after Building Custom model gRPC image:"
        df -hT
    echo "Building image transformer gRPC image"
    docker buildx build -t "${CUSTOM_TRANSFORMER_GRPC_IMG_TAG}" -f custom_transformer_grpc.Dockerfile \
      -o type=docker,dest="${DOCKER_IMAGES_PATH}/${CUSTOM_TRANSFORMER_GRPC_IMG}-${GITHUB_SHA}",compression-level=0 .
    echo "Disk usage after Building image transformer gRPC image:"
        df -hT
    echo "Building Huggingface CPU image"
    docker buildx build -t "${HUGGINGFACE_CPU_IMG_TAG}" -f huggingface_server_cpu.Dockerfile \
      -o type=docker,dest="${DOCKER_IMAGES_PATH}/${HUGGINGFACE_IMG}-${GITHUB_SHA}",compression-level=0 .
    echo "Disk usage after Building Huggingface CPU image:"
        df -hT
  fi

  if [[ " ${types[*]} " =~ "explainer" ]]; then
    echo "Building ART explainer image"
    docker buildx build -t "${ART_IMG_TAG}" -f artexplainer.Dockerfile \
      -o type=docker,dest="${DOCKER_IMAGES_PATH}/${ART_IMG}-${GITHUB_SHA}",compression-level=0 .
  fi

  if [[ " ${types[*]} " =~ "transformer" ]]; then
    echo "Building Image transformer image"
    docker buildx build -t "${IMAGE_TRANSFORMER_IMG_TAG}" -f custom_transformer.Dockerfile \
      -o type=docker,dest="${DOCKER_IMAGES_PATH}/${IMAGE_TRANSFORMER_IMG}-${GITHUB_SHA}",compression-level=0 .
  fi

popd

echo "Done building images"
