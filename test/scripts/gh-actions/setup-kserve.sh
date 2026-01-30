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

# The script will install KServe dependencies in the GH Actions environment.
# (Istio, Knative, cert-manager, kustomize, yq)
# Usage: setup-kserve.sh $DEPLOYMENT_MODE $NETWORK_LAYER

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]:-$0}")" &>/dev/null && pwd 2>/dev/null)"
source "${SCRIPT_DIR}/../../../hack/setup/common.sh"

export DEPLOYMENT_MODE="${1:-'Knative'}"
export NETWORK_LAYER="${2:-'istio'}"
export GATEWAY_NETWORK_LAYER="false"
export LLMISVC="${LLMISVC:-'false'}"

# Extract gateway class name from NETWORK_LAYER (e.g., "envoy-gatewayapi" -> "envoy")
# If NETWORK_LAYER contains "-", extract the first part; otherwise, use "false"
if [[ $NETWORK_LAYER == *"-gatewayapi"* ]]; then
  export GATEWAY_NETWORK_LAYER="${NETWORK_LAYER%%-*}"
fi

echo "Installing KServe using Kustomize..."

echo "Creating a namespace kserve-ci-test ..."
kubectl create namespace kserve-ci-e2e-test

echo "Installing KServe Python SDK ..."
pushd python/kserve >/dev/null
    uv sync --active --group test
popd


if [[ $LLMISVC == "false" ]]; then  
  KSERVE_OVERLAY_DIR=test INSTALL_RUNTIMES=false ${REPO_ROOT}/hack/setup/infra/manage.kserve-kustomize.sh 

  echo "Installing KServe Runtimes..."
  kubectl apply --server-side=true -k config/overlays/test/clusterresources

  kubectl get events -A

  echo "Add testing models to s3 storage ..."
  kubectl apply -f config/overlays/test/s3-local-backend/seaweedfs-init-job.yaml -n kserve
  kubectl wait --for=condition=complete --timeout=90s job/s3-init -n kserve

  echo "Add storageSpec testing secrets ..."
  kubectl apply -f config/overlays/test/s3-local-backend/storage-config-secret.yaml -n kserve-ci-e2e-test
else
  KSERVE_OVERLAY_DIR=test-llmisvc ${REPO_ROOT}/hack/setup/infra/manage.kserve-kustomize.sh
fi

echo "Show inferenceservice-config configmap..."
kubectl get configmap inferenceservice-config -n kserve
