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

CONTROLLER_IMG=kserve/kserve-controller:${GITHUB_SHA}
STORAGE_INIT_IMG=kserve/storage-initializer:${GITHUB_SHA}
AGENT_IMG=kserve/agent:${GITHUB_SHA}

# Predictor runtime server images
SKLEARN_IMG=kserve/sklearnserver:${GITHUB_SHA}
XGB_IMG=kserve/xgbserver:${GITHUB_SHA}
LGB_IMG=kserve/lgbserver:${GITHUB_SHA}
PMML_IMG=kserve/pmmlserver:${GITHUB_SHA}
PADDLE_IMG=kserve/paddleserver:${GITHUB_SHA}
# Explainer images
ALIBI_IMG=kserve/alibi-explainer:${GITHUB_SHA}
AIX_IMG=kserve/aix-explainer:${GITHUB_SHA}
# Transformer images
IMAGE_TRANSFORMER_IMG=kserve/image-transformer:${GITHUB_SHA}


docker build . -t ${CONTROLLER_IMG}
docker build -f agent.Dockerfile . -t ${AGENT_IMG}

pushd python >/dev/null
  docker build -t ${SKLEARN_IMG} -f sklearn.Dockerfile .
  docker build -t ${XGB_IMG} -f xgb.Dockerfile .
  docker build -t ${LGB_IMG} -f lgb.Dockerfile .
  docker build -t ${PMML_IMG} -f pmml.Dockerfile .
  # docker build -t ${PADDLE_IMG} -f paddle.Dockerfile .

  # docker build -t ${ALIBI_IMG} -f alibiexplainer.Dockerfile .
  # docker build -t ${AIX_IMG} -f aixexplainer.Dockerfile .

  docker build -t ${IMAGE_TRANSFORMER_IMG} -f custom_transformer.Dockerfile .

  docker build -t ${STORAGE_INIT_IMG} -f storage-initializer.Dockerfile .
popd

# Update KServe configurations to use the correct tag. This replaces all 'latest' entries in the configmap include the
# agent and storage-initializer.
sed -i -e "s/latest/${GITHUB_SHA}/g" config/overlays/test/configmap/inferenceservice.yaml

# Update agent tag
# sed -i -e "s/kserve\/agent:latest/kserve\/agent:${GITHUB_SHA}/g" config/overlays/test/configmap/inferenceservice.yaml

# Update storage init tag
# sed -i -e "s/kserve\/storage-initializer:latest/kserve\/storage-initializer:${GITHUB_SHA}/g" config/overlays/test/configmap/inferenceservice.yaml

# Update runtimes
sed -i -e "s/latest/${GITHUB_SHA}/g" config/overlays/test/runtimes/kustomization.yaml

# Update controller image tag
sed -i -e "s/latest/${GITHUB_SHA}/g" config/overlays/test/manager_image_patch.yaml
