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

# Install LeaderWorkerSet (LWS)
# Usage: install-operator.sh [--reinstall|--uninstall]
#   or:  REINSTALL=true install-operator.sh
#   or:  UNINSTALL=true install-operator.sh

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
    log_info "Uninstalling LeaderWorkerSet (LWS)..."
    kubectl delete -f "https://github.com/kubernetes-sigs/lws/releases/download/${LWS_VERSION}/manifests.yaml" --ignore-not-found=true 2>/dev/null || true
    log_success "LWS uninstalled"
}

install() {
    if kubectl get deployment lws-controller-manager -n lws-system &>/dev/null; then
        if [ "$REINSTALL" = false ]; then
            log_info "LWS is already installed. Use --reinstall to reinstall."
            return 0
        else
            log_info "Reinstalling LWS..."
            uninstall
        fi
    fi

    log_info "Installing LWS ${LWS_VERSION}..."
    kubectl apply --server-side -f "https://github.com/kubernetes-sigs/lws/releases/download/${LWS_VERSION}/manifests.yaml"

    log_success "Successfully installed LWS ${LWS_VERSION}"

    wait_for_pods "lws-system" "control-plane=controller-manager" "300s"

    log_success "LWS is ready!"
}

if [ "$UNINSTALL" = true ]; then
    uninstall
    exit 0
fi

install
