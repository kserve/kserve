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

# Install Gateway Inference Extension CRDs
# Usage: manage.gateway-inference-extension-crd.sh [--reinstall|--uninstall]
#   or:  REINSTALL=true manage.gateway-inference-extension-crd.sh
#   or:  UNINSTALL=true manage.gateway-inference-extension-crd.sh

# INIT
SCRIPT_DIR="$(cd "$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")" && pwd)"

source "${SCRIPT_DIR}/../../common.sh"

REINSTALL="${REINSTALL:-false}"
UNINSTALL="${UNINSTALL:-false}"

if [[ "$*" == *"--uninstall"* ]]; then
    UNINSTALL=true
elif [[ "$*" == *"--reinstall"* ]]; then
    REINSTALL=true
fi
# INIT END

uninstall() {
    log_info "Uninstalling Gateway Inference Extension CRD..."
    kubectl delete -f "https://github.com/kubernetes-sigs/gateway-api-inference-extension/releases/download/${GIE_VERSION}/v1-manifests.yaml" --ignore-not-found=true 2>/dev/null || true
    log_info "Uninstalling llm-d CRDs from llm-d-router ${LLMD_ROUTER_VERSION}..."
    kubectl delete -f "https://github.com/llm-d/llm-d-router/releases/download/${LLMD_ROUTER_VERSION}/manifests.yaml" --ignore-not-found=true 2>/dev/null || true
    log_success "Gateway Inference Extension CRDs uninstalled"
}

install() {
    if kubectl get crd inferencepools.inference.networking.k8s.io &>/dev/null; then
        if [ "$REINSTALL" = false ]; then
            log_info "Gateway Inference Extension CRDs are already installed. Use --reinstall to reinstall."
            return 0
        else
            log_info "Reinstalling Gateway Inference Extension CRDs..."
            uninstall
        fi
    fi

    log_info "Installing Gateway Inference Extension CRD ${GIE_VERSION}..."
    kubectl apply -f "https://github.com/kubernetes-sigs/gateway-api-inference-extension/releases/download/${GIE_VERSION}/v1-manifests.yaml"

    log_success "Successfully installed Gateway Inference Extension CRDs ${GIE_VERSION}"

    log_info "Installing llm-d.ai CRDs from llm-d-router ${LLMD_ROUTER_VERSION}..."
    kubectl apply -f "https://github.com/llm-d/llm-d-router/releases/download/${LLMD_ROUTER_VERSION}/manifests.yaml"

    wait_for_crds "60s" \
        "inferencepools.inference.networking.k8s.io" \
        "inferenceobjectives.llm-d.ai" \
        "inferencemodelrewrites.llm-d.ai"

    log_success "Gateway Inference Extension CRDs are ready!"
}

if [ "$UNINSTALL" = true ]; then
    uninstall
    exit 0
fi

install
