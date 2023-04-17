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
IFS=,
types=($1)

# Predictor runtime server images
SKLEARN_IMG=kserve/sklearnserver:${GITHUB_SHA}
XGB_IMG=kserve/xgbserver:${GITHUB_SHA}
LGB_IMG=kserve/lgbserver:${GITHUB_SHA}
PMML_IMG=kserve/pmmlserver:${GITHUB_SHA}
PADDLE_IMG=kserve/paddleserver:${GITHUB_SHA}
CUSTOM_MODEL_GRPC=kserve/custom-model-grpc:${GITHUB_SHA}
CUSTOM_TRANSFORMER_GRPC=kserve/custom-image-transformer-grpc:${GITHUB_SHA}
# Explainer images
ALIBI_IMG=kserve/alibi-explainer:${GITHUB_SHA}
ART_IMG=kserve/art-explainer:${GITHUB_SHA}
# Transformer images
IMAGE_TRANSFORMER_IMG=kserve/image-transformer:${GITHUB_SHA}


pushd python >/dev/null
  if [[ " ${types[*]} " =~ "predictor" ]]; then
    echo "Building Sklearn image"
    docker build -t ${SKLEARN_IMG} -f sklearn.Dockerfile .
    echo "Building XGB image"
    docker build -t ${XGB_IMG} -f xgb.Dockerfile .
    echo "Building LGB image"
    docker build -t ${LGB_IMG} -f lgb.Dockerfile .
    echo "Building PMML image"
    docker build -t ${PMML_IMG} -f pmml.Dockerfile .
    echo "Building Paddle image"
    docker build -t ${PADDLE_IMG} -f paddle.Dockerfile .
    echo "Building Custom model gRPC image"
    docker build -t ${CUSTOM_MODEL_GRPC} -f custom_model_grpc.Dockerfile .
    echo "Building image transformer gRPC image"
    docker build -t ${CUSTOM_TRANSFORMER_GRPC} -f custom_transformer_grpc.Dockerfile .
  fi

  if [[ " ${types[*]} " =~ "explainer" ]]; then
    echo "Building Alibi image"
    docker build -t ${ALIBI_IMG} -f alibiexplainer.Dockerfile .
    echo "Building ART explainer image"
    docker build -t ${ART_IMG} -f artexplainer.Dockerfile .
    
    echo "Pruning images to free up space"
    docker image prune -f

  fi

  if [[ " ${types[*]} " =~ "transformer" ]]; then
    echo "Building Image transformer image"
    docker build -t ${IMAGE_TRANSFORMER_IMG} -f custom_transformer.Dockerfile .
  fi

popd

echo "Done building images"
