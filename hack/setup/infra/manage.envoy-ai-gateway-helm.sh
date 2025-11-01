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
    VERSION_NUMBER="${ENVOY_AI_GATEWAY_VERSION#v}"
    kubectl delete -f "https://raw.githubusercontent.com/envoyproxy/ai-gateway/v${VERSION_NUMBER}/examples/inference-pool/config.yaml" --ignore-not-found=true --force --grace-period=0 2>/dev/null || true
    kubectl delete -f "https://raw.githubusercontent.com/envoyproxy/ai-gateway/v${VERSION_NUMBER}/manifests/envoy-gateway-config/rbac.yaml" --ignore-not-found=true --force --grace-period=0 2>/dev/null || true
    kubectl delete -f "https://raw.githubusercontent.com/envoyproxy/ai-gateway/v${VERSION_NUMBER}/manifests/envoy-gateway-config/config.yaml" --ignore-not-found=true --force --grace-period=0 2>/dev/null || true
    kubectl delete -f "https://raw.githubusercontent.com/envoyproxy/ai-gateway/v${VERSION_NUMBER}/manifests/envoy-gateway-config/redis.yaml" --ignore-not-found=true --force --grace-period=0 2>/dev/null || true
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

    log_info "Installing Envoy AI Gateway CRDs ${ENVOY_AI_GATEWAY_VERSION}..."
    helm install aieg-crd oci://docker.io/envoyproxy/ai-gateway-crds-helm \
        --version "${ENVOY_AI_GATEWAY_VERSION}" \
        --namespace envoy-ai-gateway-system \
        --create-namespace

    log_info "Installing Envoy AI Gateway ${ENVOY_AI_GATEWAY_VERSION}..."
    helm install aieg oci://docker.io/envoyproxy/ai-gateway-helm \
        --version "${ENVOY_AI_GATEWAY_VERSION}" \
        --namespace envoy-ai-gateway-system \
        --create-namespace

    wait_for_deployment "envoy-ai-gateway-system" "ai-gateway-controller" "180s"
    log_success "Successfully installed Envoy AI Gateway ${ENVOY_AI_GATEWAY_VERSION} via Helm"
    
    log_info "Configuring Envoy Gateway for AI Gateway integration..."
    VERSION_NUMBER="${ENVOY_AI_GATEWAY_VERSION#v}"
    kubectl apply -f "https://raw.githubusercontent.com/envoyproxy/ai-gateway/v${VERSION_NUMBER}/manifests/envoy-gateway-config/redis.yaml"
    kubectl apply -f "https://raw.githubusercontent.com/envoyproxy/ai-gateway/v${VERSION_NUMBER}/manifests/envoy-gateway-config/config.yaml"
    kubectl apply -f "https://raw.githubusercontent.com/envoyproxy/ai-gateway/v${VERSION_NUMBER}/manifests/envoy-gateway-config/rbac.yaml"

    log_info "Enabling Gateway API Inference Extension support for Envoy Gateway..."
    kubectl apply -f "https://raw.githubusercontent.com/envoyproxy/ai-gateway/v${VERSION_NUMBER}/examples/inference-pool/config.yaml"
    kubectl rollout restart -n envoy-gateway-system deployment/envoy-gateway    
    wait_for_deployment "envoy-gateway-system" "envoy-gateway" "180s"
    log_success "Envoy AI Gateway is ready!"
}

if [ "$UNINSTALL" = true ]; then
    uninstall
    exit 0
fi

install
