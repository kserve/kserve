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
# Usage: setup-deps.sh $DEPLOYMENT_MODE $NETWORK_LAYER

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]:-$0}")" &>/dev/null && pwd 2>/dev/null)"
DEPLOYMENT_MODE="${1:-'serverless'}"
NETWORK_LAYER="${2:-'istio'}"
ENABLE_KEDA="${3:-'false'}"

ISTIO_VERSION="1.23.2"
CERT_MANAGER_VERSION="v1.16.1"
YQ_VERSION="v4.28.1"
GATEWAY_API_VERSION="v1.2.1"
ENVOY_GATEWAY_VERSION="v1.2.2"

echo "Installing yq ..."
wget https://github.com/mikefarah/yq/releases/download/${YQ_VERSION}/yq_linux_amd64 -O /usr/local/bin/yq && chmod +x /usr/local/bin/yq

if [[ $NETWORK_LAYER == "istio-gatewayapi" || $NETWORK_LAYER == "envoy-gatewayapi" ]]; then
  echo "Installing Gateway CRDs ..."
  kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/${GATEWAY_API_VERSION}/standard-install.yaml
fi

if [[ $NETWORK_LAYER == "istio-ingress" || $NETWORK_LAYER == "istio-gatewayapi" || $NETWORK_LAYER == "istio" ]]; then
  echo "Installing Istio ..."
  mkdir istio_tmp
  pushd istio_tmp >/dev/null
  curl -L https://istio.io/downloadIstio | ISTIO_VERSION=${ISTIO_VERSION} sh -
  cd istio-${ISTIO_VERSION}
  export PATH=$PWD/bin:$PATH
  istioctl manifest generate --set meshConfig.accessLogFile=/dev/stdout >${SCRIPT_DIR}/../../overlays/istio/generated-manifest.yaml
  popd
  kubectl create ns istio-system
  for i in {1..3}; do kubectl apply -k test/overlays/istio && break || sleep 15; done

  echo "Waiting for Istio to be ready ..."
  kubectl wait --for=condition=Ready pods --all --timeout=240s -n istio-system
elif [[ $NETWORK_LAYER == "envoy-gatewayapi" ]]; then
  echo "Installing Envoy Gateway ..."
  helm install eg oci://docker.io/envoyproxy/gateway-helm --version ${ENVOY_GATEWAY_VERSION} -n envoy-gateway-system --create-namespace --wait
  kubectl wait --timeout=5m -n envoy-gateway-system deployment/envoy-gateway --for=condition=Available

  echo "Creating envoy GatewayClass ..."
  cat <<EOF | kubectl apply -f -
  apiVersion: gateway.networking.k8s.io/v1
  kind: GatewayClass
  metadata:
    name: envoy
  spec:
    controllerName: gateway.envoyproxy.io/gatewayclass-controller  
EOF
fi

if [[ $NETWORK_LAYER == "istio-ingress" ]]; then
  echo "Creating istio ingress class"
  cat <<EOF | kubectl apply -f -
  apiVersion: networking.k8s.io/v1
  kind: IngressClass
  metadata:
    name: istio
  spec:
    controller: istio.io/ingress-controller
EOF
fi

shopt -s nocasematch
if [[ $DEPLOYMENT_MODE == "serverless" ]]; then
  # Serverless mode
  source ./test/scripts/gh-actions/install-knative-operator.sh
  echo "Installing Knative serving ..."
  kubectl apply -f ./test/overlays/knative/knative-serving-istio.yaml
  echo "Waiting for Knative to be ready ..."
  kubectl wait --for=condition=Ready -n knative-serving KnativeServing knative-serving --timeout=300s
  # echo "Add knative hpa..."
  # kubectl apply -f https://github.com/knative/serving/releases/download/knative-v1.0.0/serving-hpa.yaml
fi
shopt -u nocasematch

if [[ $DEPLOYMENT_MODE == "raw" ]]; then
  if [[ $ENABLE_KEDA == "true" ]]; then
    echo "Installing KEDA ..."
    kubectl apply -f ./test/overlays/keda/keda.yaml
    kubectl apply -f ./test/overlays/opentelemetry/opentelemetry-operator.yaml
  fi
fi

echo "Installing cert-manager ..."
kubectl create namespace cert-manager
sleep 2
kubectl apply --validate=false -f https://github.com/jetstack/cert-manager/releases/download/${CERT_MANAGER_VERSION}/cert-manager.yaml

echo "Waiting for cert-manager to be ready ..."
kubectl wait --for=condition=ready pod -l 'app in (cert-manager,webhook)' --timeout=180s -n cert-manager
