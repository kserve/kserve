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

# Install Gateway API CRDs
# Usage: install-crd.sh [--reinstall|--uninstall]
#   or:  REINSTALL=true install-crd.sh
#   or:  UNINSTALL=true install-crd.sh

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

uninstall() {
    log_info "Uninstalling Gateway API CRDs..."
    kubectl delete -f "https://github.com/kubernetes-sigs/gateway-api/releases/download/${GATEWAY_API_VERSION}/standard-install.yaml" --ignore-not-found=true 2>/dev/null || true
    log_success "Gateway API CRDs uninstalled"
}

install() {
    if kubectl get crd gateways.gateway.networking.k8s.io &>/dev/null; then
        if [ "$REINSTALL" = false ]; then
            log_info "Gateway API CRDs are already installed. Use --reinstall to reinstall."
            return 0
        else
            log_info "Reinstalling Gateway API CRDs..."
            uninstall
        fi
    fi

    log_info "Installing Gateway API CRDs ${GATEWAY_API_VERSION}..."
    kubectl apply -f "https://github.com/kubernetes-sigs/gateway-api/releases/download/${GATEWAY_API_VERSION}/standard-install.yaml"

    log_success "Successfully installed Gateway API CRDs ${GATEWAY_API_VERSION}"

    wait_for_crds "60s" \
        "gateways.gateway.networking.k8s.io" \
        "gatewayclasses.gateway.networking.k8s.io"

    log_success "Gateway API CRDs are ready!"
}

if [ "$UNINSTALL" = true ]; then
    uninstall
    exit 0
fi

install
