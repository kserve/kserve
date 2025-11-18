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
source "${SCRIPT_DIR}/../../../hack/setup/common.sh"
REPO_ROOT="$(find_repo_root "${SCRIPT_DIR}")"

source "${REPO_ROOT}/kserve-deps.env"

DEPLOYMENT_MODE="${1:-'serverless'}"
NETWORK_LAYER="${2:-'istio'}"
ENABLE_KEDA="${3:-'false'}"
LLMISVC="${4:-'false'}"

# Parse network layer configuration
USES_GATEWAY_API=false
USES_ISTIO=false
USES_ENVOY=false
USES_ISTIO_INGRESS=false

case "$NETWORK_LAYER" in
  istio-gatewayapi)
    USES_GATEWAY_API=true
    USES_ISTIO=true
    ;;
  envoy-gatewayapi)
    USES_GATEWAY_API=true
    USES_ENVOY=true
    ;;
  istio-ingress)
    USES_ISTIO=true
    USES_ISTIO_INGRESS=true
    ;;
  istio)
    USES_ISTIO=true
    ;;
esac

${REPO_ROOT}/hack/setup/cli/install-yq.sh
${REPO_ROOT}/hack/setup/infra/manage.cert-manager-helm.sh

# Install Gateway API CRDs if needed
if [[ $USES_GATEWAY_API == true ]]; then
  ${REPO_ROOT}/hack/setup/infra/gateway-api/manage.gateway-api-crd.sh
fi

# Install Istio with minimal resources for CI/test environment
if [[ $USES_ISTIO == true ]]; then
  export ISTIOD_EXTRA_ARGS="--set resources.requests.cpu=5m --set resources.requests.memory=32Mi --set meshConfig.accessLogFile=/dev/stdout"
  export ISTIO_GATEWAY_EXTRA_ARGS="--set resources.requests.cpu=5m --set resources.requests.memory=32Mi --set resources.limits.cpu=100m --set resources.limits.memory=128Mi"
  ${REPO_ROOT}/hack/setup/infra/manage.istio-helm.sh
fi

# Install Envoy Gateway
if [[ $USES_ENVOY == true ]]; then
  export GATEWAY_NETWORK_LAYER="${NETWORK_LAYER%%-*}"
  ${REPO_ROOT}/hack/setup/infra/manage.envoy-gateway-helm.sh
  ${REPO_ROOT}/hack/setup/infra/gateway-api/manage.gateway-api-gwclass.sh
fi

# Install Istio IngressClass
if [[ $USES_ISTIO_INGRESS == true ]]; then
  ${REPO_ROOT}/hack/setup/infra/manage.istio-ingress-class.sh
fi

# Install LLM-specific components
if [[ $LLMISVC == "true" ]]; then
  ${REPO_ROOT}/hack/setup/infra/manage.lws-operator.sh
fi

# Install KServe Gateway for Gateway API or LLM use cases
if [[ $USES_GATEWAY_API == true ]] || [[ $LLMISVC == "true" ]]; then
  export GATEWAYCLASS_NAME="${NETWORK_LAYER%%-*}"
  ${REPO_ROOT}/hack/setup/infra/gateway-api/manage.gateway-api-gw.sh
fi

shopt -s nocasematch
if [[ $DEPLOYMENT_MODE == "serverless" ]] || [[ $DEPLOYMENT_MODE == "Knative" ]]; then
  # Serverless mode - Install Knative Operator and Serving (Istio network layer)
  echo "Installing Knative Operator and Serving...(NETWORK_LAYER: ${NETWORK_LAYER})"  
  NETWORK_LAYER="${NETWORK_LAYER}" ${REPO_ROOT}/hack/setup/infra/knative/manage.knative-operator-helm.sh
fi
shopt -u nocasematch

if [[ $DEPLOYMENT_MODE == "raw" ]]; then
  if [[ $ENABLE_KEDA == "true" ]]; then
    echo "KEDA and OpenTelemetry will be installed via Helm later in the script..."
    echo "Installing KEDA and OpenTelemetry components..."

    # Install KEDA
    ${REPO_ROOT}/hack/setup/infra/manage.keda-helm.sh

    # Install OpenTelemetry Operator with specific collector image
    export OTEL_OPERATOR_EXTRA_ARGS="--set manager.collectorImage.repository=otel/opentelemetry-collector-contrib"
    ${REPO_ROOT}/hack/setup/infra/manage.opentelemetry-helm.sh

    # Install KEDA OTel add-on with validating admission policy disabled
    export KEDA_OTEL_ADDON_EXTRA_ARGS="--set validatingAdmissionPolicy.enabled=false"
    ${REPO_ROOT}/hack/setup/infra/manage.keda-otel-addon-helm.sh
  fi
fi
