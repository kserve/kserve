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

# The script will install KServe with LLM controller in the GH Actions environment.
# (Istio, Knative, cert-manager, kustomize, yq)
# Usage: setup-kserve-llm.sh $DEPLOYMENT_MODE $NETWORK_LAYER

set -o errexit
set -o nounset
set -o pipefail
DEPLOYMENT_MODE="${1:-'serverless'}"
NETWORK_LAYER="${2:-'istio'}"

echo "🏗️ Setting up dependencies (Gateway API, Istio, cert-manager)..."
./test/scripts/gh-actions/setup-deps.sh "$DEPLOYMENT_MODE" "$NETWORK_LAYER" "false" "false"

echo "🏗️ Deploying KServe with LLM controller using make deploy-ci-llm..."
make deploy-ci-llm

shopt -s nocasematch
if [[ $DEPLOYMENT_MODE == "raw" ]];then
  echo "Patching default deployment mode to raw deployment"
  kubectl patch cm -n kserve inferenceservice-config --patch='{"data": {"deploy": "{\"defaultDeploymentMode\": \"RawDeployment\"}"}}'

  if [[ $NETWORK_LAYER == "envoy-gatewayapi" ]]; then
    echo "Creating Envoy Gateway ..."
    kubectl apply -f config/overlays/test/gateway/ingress_gateway.yaml
    sleep 10
    echo "Waiting for envoy gateway to be ready ..."
    kubectl wait --timeout=5m -n envoy-gateway-system pod -l serving.kserve.io/gateway=kserve-ingress-gateway --for=condition=Ready
  elif [[ $NETWORK_LAYER == "istio-gatewayapi" ]]; then
    echo "Creating Istio Gateway ..."
    # Replace gatewayclass name
    kubectl apply -f - <<EOF
$(sed 's/envoy/istio/g' config/overlays/test/gateway/ingress_gateway.yaml)
EOF
    sleep 10
    echo "Waiting for istio gateway to be ready ..."
    kubectl wait --timeout=5m -n kserve pod -l serving.kserve.io/gateway=kserve-ingress-gateway --for=condition=Ready
  fi
fi
shopt -u nocasematch

echo "🔧 Installing Gateway API Inference Extension CRDs..."
# Install required CRDs for InferenceModel and InferencePool
kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/gateway-api-inference-extension/v0.5.0/config/crd/bases/inference.networking.x-k8s.io_inferencemodels.yaml
kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/gateway-api-inference-extension/v0.5.0/config/crd/bases/inference.networking.x-k8s.io_inferencepools.yaml

# Wait for CRDs to be established
kubectl wait --for=condition=established --timeout=60s crd/inferencemodels.inference.networking.x-k8s.io
kubectl wait --for=condition=established --timeout=60s crd/inferencepools.inference.networking.x-k8s.io

echo "✅ Gateway API Inference Extension CRDs installed"

echo "Waiting for KServe and LLM controller started ..."
kubectl wait --for=condition=Ready pods --all --timeout=180s -n kserve
kubectl get events -A

echo "Add testing models to minio storage ..."
kubectl apply -f config/overlays/test/minio/minio-init-job.yaml -n kserve
kubectl wait --for=condition=complete --timeout=90s job/minio-init -n kserve

echo "Creating a namespace kserve-ci-e2e-test ..."
kubectl create namespace kserve-ci-e2e-test

echo "Add storageSpec testing secrets ..."
kubectl apply -f config/overlays/test/minio/minio-user-secret.yaml -n kserve-ci-e2e-test

echo "Installing KServe Python SDK ..."
pushd python/kserve >/dev/null
    uv sync --active --group test
popd
