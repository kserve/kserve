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
ENABLE_LWS="${4:-'false'}"

${REPO_ROOT}/hack/setup/cli/install-yq.sh

if [[ $NETWORK_LAYER == "istio-gatewayapi" || $NETWORK_LAYER == "envoy-gatewayapi" ]]; then
  ${REPO_ROOT}/hack/setup/infra/manage.gateway-api-crd.sh
fi

if [[ $NETWORK_LAYER == "istio-ingress" || $NETWORK_LAYER == "istio-gatewayapi" || $NETWORK_LAYER == "istio" ]]; then
  # Set minimal resources for CI/test environment (matching previous overlays)
  export ISTIOD_EXTRA_ARGS="--set resources.requests.cpu=5m --set resources.requests.memory=32Mi --set meshConfig.accessLogFile=/dev/stdout"
  export ISTIO_GATEWAY_EXTRA_ARGS="--set resources.requests.cpu=5m --set resources.requests.memory=32Mi --set resources.limits.cpu=100m --set resources.limits.memory=128Mi"

  # Use the new helm-based installation script
  ${REPO_ROOT}/hack/setup/infra/manage.istio-helm.sh
elif [[ $NETWORK_LAYER == "envoy-gatewayapi" ]]; then
  ${REPO_ROOT}/hack/setup/infra/manage.envoy-gateway-helm.sh
fi

if [[ $NETWORK_LAYER == "istio-ingress" ]]; then
  ${REPO_ROOT}/hack/setup/infra/manage.istio-ingress-class.sh
fi

shopt -s nocasematch
if [[ $DEPLOYMENT_MODE == "serverless" ]]; then
  # Serverless mode - Install Knative Operator and Serving (Istio network layer)
  echo "Installing Knative Operator and Serving..."
  ${REPO_ROOT}/hack/setup/infra/knative/manage.knative-operator-helm.sh
fi
shopt -u nocasematch

if [[ $DEPLOYMENT_MODE == "raw" ]]; then
  if [[ $ENABLE_KEDA == "true" ]]; then
    echo "KEDA and OpenTelemetry will be installed via Helm later in the script..."
  fi
fi

if [[ $ENABLE_LWS == "true" ]]; then
  ${REPO_ROOT}/hack/setup/infra/manage.lws-operator.sh
fi

${REPO_ROOT}/hack/setup/infra/manage.cert-manager-helm.sh

if [[ $DEPLOYMENT_MODE == "raw" ]]; then
  if [[ $ENABLE_KEDA == "true" ]]; then
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
