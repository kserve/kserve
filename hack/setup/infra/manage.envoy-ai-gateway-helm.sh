#!/bin/bash

# Copyright 2025 The KServe Authors.
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

# Install Envoy AI Gateway using Helm
# Usage: install-helm.sh [--reinstall|--uninstall]
#   or:  REINSTALL=true install-helm.sh
#   or:  UNINSTALL=true install-helm.sh

# INIT
SCRIPT_DIR="$(cd "$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")" && pwd)"

source "${SCRIPT_DIR}/../common.sh"

REINSTALL="${REINSTALL:-false}"
UNINSTALL="${UNINSTALL:-false}"

if [[ "$*" == *"--uninstall"* ]]; then
    UNINSTALL=true
elif [[ "$*" == *"--reinstall"* ]]; then
    REINSTALL=true
fi
# INIT END

check_cli_exist helm

uninstall() {
    log_info "Uninstalling Envoy AI Gateway..."
    helm uninstall aieg -n envoy-ai-gateway-system 2>/dev/null || true
    helm uninstall aieg-crd -n envoy-ai-gateway-system 2>/dev/null || true
    kubectl delete all --all -n envoy-ai-gateway-system --force --grace-period=0 2>/dev/null || true
    kubectl delete namespace envoy-ai-gateway-system --wait=true --timeout=60s --force --grace-period=0 2>/dev/null || true
    kubectl delete all --all -n redis-system --force --grace-period=0 2>/dev/null || true
    kubectl delete namespace redis-system --wait=true --timeout=60s --force --grace-period=0 2>/dev/null || true
    log_success "Envoy AI Gateway uninstalled"
}

install() {
    if helm list -n envoy-ai-gateway-system 2>/dev/null | grep -q "aieg"; then
        if [ "$REINSTALL" = false ]; then
            log_info "Envoy AI Gateway is already installed. Use --reinstall to reinstall."
            return 0
        else
            log_info "Reinstalling Envoy AI Gateway..."
            uninstall
        fi
    fi

    log_info "Updating Envoy Gateway ${ENVOY_GATEWAY_VERSION}...to add inference pool addons for Envoy AI Gateway"
    helm upgrade -i eg oci://docker.io/envoyproxy/gateway-helm \
        --version "${ENVOY_GATEWAY_VERSION}" \
        -n envoy-gateway-system \
        --create-namespace \
        -f https://raw.githubusercontent.com/envoyproxy/ai-gateway/${ENVOY_AI_GATEWAY_VERSION}/manifests/envoy-gateway-values.yaml \
        -f https://raw.githubusercontent.com/envoyproxy/ai-gateway/${ENVOY_AI_GATEWAY_VERSION}/examples/inference-pool/envoy-gateway-values-addon.yaml \
        --wait
    
    log_success "Successfully Updated Envoy Gateway ${ENVOY_GATEWAY_VERSION} for Envoy AI Gateway"

    log_info "Installing Envoy AI Gateway CRDs ${ENVOY_AI_GATEWAY_VERSION}..."
    helm upgrade -i aieg-crd oci://docker.io/envoyproxy/ai-gateway-crds-helm \
        --version "${ENVOY_AI_GATEWAY_VERSION}" \
        --namespace envoy-ai-gateway-system \
        --create-namespace

    log_info "Installing Envoy AI Gateway ${ENVOY_AI_GATEWAY_VERSION}..."
    helm upgrade -i aieg oci://docker.io/envoyproxy/ai-gateway-helm \
        --version "${ENVOY_AI_GATEWAY_VERSION}" \
        --namespace envoy-ai-gateway-system \
        --create-namespace

    kubectl wait --timeout=2m -n envoy-ai-gateway-system deployment/ai-gateway-controller --for=condition=Available
    log_success "Envoy AI Gateway ${ENVOY_AI_GATEWAY_VERSION} is ready!"
}

if [ "$UNINSTALL" = true ]; then
    uninstall
    exit 0
fi

install
