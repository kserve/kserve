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
# Explainer images
ALIBI_IMG_TAG=${DOCKER_REPO}/${ALIBI_IMG}:${GITHUB_SHA}
ART_IMG_TAG=${DOCKER_REPO}/${ART_IMG}:${GITHUB_SHA}
# Transformer images
IMAGE_TRANSFORMER_IMG_TAG=${DOCKER_REPO}/${IMAGE_TRANSFORMER_IMG}:${GITHUB_SHA}
types=("$1")

pushd python >/dev/null
  if [[ " ${types[*]} " =~ "predictor" ]]; then
    echo "Building Sklearn image"
    docker buildx build -t "${SKLEARN_IMG_TAG}" -f sklearn.Dockerfile .
    docker image save -o "${DOCKER_IMAGES_PATH}/${SKLEARN_IMG}-${GITHUB_SHA}" "${SKLEARN_IMG_TAG}"
    echo "Building XGB image"
    docker buildx build -t "${XGB_IMG_TAG}" -f xgb.Dockerfile .
    docker image save -o "${DOCKER_IMAGES_PATH}/${XGB_IMG}-${GITHUB_SHA}" "${XGB_IMG_TAG}"
    echo "Building LGB image"
    docker buildx build -t "${LGB_IMG_TAG}" -f lgb.Dockerfile .
    docker image save -o "${DOCKER_IMAGES_PATH}/${LGB_IMG}-${GITHUB_SHA}" "${LGB_IMG_TAG}"
    echo "Building PMML image"
    docker buildx build -t "${PMML_IMG_TAG}" -f pmml.Dockerfile .
    docker image save -o "${DOCKER_IMAGES_PATH}/${PMML_IMG}-${GITHUB_SHA}" "${PMML_IMG_TAG}"
    echo "Building Paddle image"
    docker buildx build -t "${PADDLE_IMG_TAG}" -f paddle.Dockerfile .
    docker image save -o "${DOCKER_IMAGES_PATH}/${PADDLE_IMG}-${GITHUB_SHA}" "${PADDLE_IMG_TAG}"
    echo "Building Custom model gRPC image"
    docker buildx build -t "${CUSTOM_MODEL_GRPC_IMG_TAG}" -f custom_model_grpc.Dockerfile .
    docker image save -o "${DOCKER_IMAGES_PATH}/${CUSTOM_MODEL_GRPC_IMG}-${GITHUB_SHA}" "${CUSTOM_MODEL_GRPC_IMG_TAG}"
    echo "Building image transformer gRPC image"
    docker buildx build -t "${CUSTOM_TRANSFORMER_GRPC_IMG_TAG}" -f custom_transformer_grpc.Dockerfile .
    docker image save -o "${DOCKER_IMAGES_PATH}/${CUSTOM_TRANSFORMER_GRPC_IMG}-${GITHUB_SHA}" "${CUSTOM_TRANSFORMER_GRPC_IMG_TAG}"
  fi

  if [[ " ${types[*]} " =~ "explainer" ]]; then
    echo "Building Alibi image"
    docker buildx build -t "${ALIBI_IMG_TAG}" -f alibiexplainer.Dockerfile .
    docker save -o "${DOCKER_IMAGES_PATH}/${ALIBI_IMG}-${GITHUB_SHA}" "${ALIBI_IMG_TAG}"
    echo "Building ART explainer image"
    docker buildx build -t "${ART_IMG_TAG}" -f artexplainer.Dockerfile .
    docker save -o "${DOCKER_IMAGES_PATH}/${ART_IMG}-${GITHUB_SHA}" "${ART_IMG_TAG}"
  fi

  if [[ " ${types[*]} " =~ "transformer" ]]; then
    echo "Building Image transformer image"
    docker buildx build -t "${IMAGE_TRANSFORMER_IMG_TAG}" -f custom_transformer.Dockerfile .
    docker save -o "${DOCKER_IMAGES_PATH}/${IMAGE_TRANSFORMER_IMG}-${GITHUB_SHA}" "${IMAGE_TRANSFORMER_IMG_TAG}"
  fi

popd

echo "Done building images"
