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

# Load image configurations
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd)"
source "${PROJECT_ROOT}/kserve-images.sh"
source "${SCRIPT_DIR}/image-cache.sh"

# Default image basename if kserve-images.sh predates AUTOGLUON_IMG (matches kserve-images.env).
AUTOGLUON_IMG="${AUTOGLUON_IMG:-autogluonserver}"

if [ -d "${DOCKER_IMAGES_PATH}" ]; then
  mkdir -p "${DOCKER_IMAGES_PATH}"  
fi

echo "Github SHA ${TAG}"

# Predictor runtime server images
SKLEARN_IMG_TAG=${KO_DOCKER_REPO}/${SKLEARN_IMG}:${TAG}
XGB_IMG_TAG=${KO_DOCKER_REPO}/${XGB_IMG}:${TAG}
LGB_IMG_TAG=${KO_DOCKER_REPO}/${LGB_IMG}:${TAG}
PMML_IMG_TAG=${KO_DOCKER_REPO}/${PMML_IMG}:${TAG}
PADDLE_IMG_TAG=${KO_DOCKER_REPO}/${PADDLE_IMG}:${TAG}
AUTOGLUON_IMG_TAG=${KO_DOCKER_REPO}/${AUTOGLUON_IMG}:${TAG}
CUSTOM_MODEL_GRPC_IMG_TAG=${KO_DOCKER_REPO}/${CUSTOM_MODEL_GRPC_IMG}:${TAG}
CUSTOM_TRANSFORMER_GRPC_IMG_TAG=${KO_DOCKER_REPO}/${CUSTOM_TRANSFORMER_GRPC_IMG}:${TAG}
HUGGINGFACE_CPU_IMG_TAG=${KO_DOCKER_REPO}/${HUGGINGFACE_IMG}:${TAG}
PREDICTIVE_IMG_TAG=${KO_DOCKER_REPO}/${PREDICTIVE_IMG}:${TAG}
# Explainer images
ART_IMG_TAG=${KO_DOCKER_REPO}/${ART_IMG}:${TAG}
# Transformer images
IMAGE_TRANSFORMER_IMG_TAG=${KO_DOCKER_REPO}/${IMAGE_TRANSFORMER_IMG}:${TAG}
types=("${1:-predictor}")

if [[ " ${types[*]} " =~ "predictor" ]]; then
  echo "Building Sklearn image"
  build_image sklearnserver "${SKLEARN_IMG_TAG}" "${DOCKER_IMAGES_PATH}/${SKLEARN_IMG}-${TAG}"
  echo "Disk usage after Building Sklearn image:"
      df -hT
  echo "Building XGB image"
  build_image xgbserver "${XGB_IMG_TAG}" "${DOCKER_IMAGES_PATH}/${XGB_IMG}-${TAG}"
  echo "Disk usage after Building XGB image:"
      df -hT
  echo "Building LGB image"
  build_image lgbserver "${LGB_IMG_TAG}" "${DOCKER_IMAGES_PATH}/${LGB_IMG}-${TAG}"
  echo "Disk usage after Building LGB image:"
      df -hT
  echo "Building PMML image"
  build_image pmmlserver "${PMML_IMG_TAG}" "${DOCKER_IMAGES_PATH}/${PMML_IMG}-${TAG}"
  echo "Disk usage after Building PMML image:"
      df -hT
  echo "Building Paddle image"
  build_image paddleserver "${PADDLE_IMG_TAG}" "${DOCKER_IMAGES_PATH}/${PADDLE_IMG}-${TAG}"
  echo "Disk usage after Building Paddle image:"
      df -hT
  echo "Building Custom model gRPC image"
  build_image custom-model-grpc "${CUSTOM_MODEL_GRPC_IMG_TAG}" "${DOCKER_IMAGES_PATH}/${CUSTOM_MODEL_GRPC_IMG}-${TAG}"
  echo "Disk usage after Building Custom model gRPC image:"
      df -hT
  echo "Building image transformer gRPC image"
  build_image custom-image-transformer-grpc "${CUSTOM_TRANSFORMER_GRPC_IMG_TAG}" "${DOCKER_IMAGES_PATH}/${CUSTOM_TRANSFORMER_GRPC_IMG}-${TAG}"
  echo "Disk usage after Building image transformer gRPC image:"
      df -hT
  echo "Building Huggingface CPU image"
  build_image huggingfaceserver "${HUGGINGFACE_CPU_IMG_TAG}" "${DOCKER_IMAGES_PATH}/${HUGGINGFACE_IMG}-${TAG}"
  echo "Disk usage after Building Huggingface CPU image:"
      df -hT
  echo "Building Predictive server image"
  build_image predictiveserver "${PREDICTIVE_IMG_TAG}" "${DOCKER_IMAGES_PATH}/${PREDICTIVE_IMG}-${TAG}"
  echo "Disk usage after Building Predictive server image:"
      df -hT
  echo "Building AutoGluon server image"
  build_image autogluonserver "${AUTOGLUON_IMG_TAG}" "${DOCKER_IMAGES_PATH}/${AUTOGLUON_IMG}-${TAG}"
  echo "Disk usage after Building AutoGluon server image:"
      df -hT
fi

if [[ " ${types[*]} " =~ "explainer" ]]; then
  echo "Building ART explainer image"
  build_image art-explainer "${ART_IMG_TAG}" "${DOCKER_IMAGES_PATH}/${ART_IMG}-${TAG}"
fi

if [[ " ${types[*]} " =~ "transformer" ]]; then
  echo "Building Image transformer image"
  build_image image-transformer "${IMAGE_TRANSFORMER_IMG_TAG}" "${DOCKER_IMAGES_PATH}/${IMAGE_TRANSFORMER_IMG}-${TAG}"
fi


echo "Done building images"
