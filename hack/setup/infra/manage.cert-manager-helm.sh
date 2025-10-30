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

# Install cert-manager using Helm
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
    log_info "Uninstalling cert-manager..."
    helm uninstall cert-manager -n cert-manager 2>/dev/null || true
    kubectl delete all --all -n cert-manager --force --grace-period=0 2>/dev/null || true
    kubectl delete namespace cert-manager --wait=true --timeout=60s --force --grace-period=0 2>/dev/null || true
    log_success "cert-manager uninstalled"
}

install() {
    if helm list -n cert-manager 2>/dev/null | grep -q "cert-manager"; then
        if [ "$REINSTALL" = false ]; then
            log_info "cert-manager is already installed. Use --reinstall to reinstall."
            return 0
        else
            log_info "Reinstalling cert-manager..."
            uninstall
        fi
    fi

    log_info "Adding cert-manager Helm repository..."
    helm repo add jetstack https://charts.jetstack.io --force-update

    log_info "Installing cert-manager ${CERT_MANAGER_VERSION}..."
    helm install \
        cert-manager jetstack/cert-manager \
        --namespace cert-manager \
        --create-namespace \
        --version "${CERT_MANAGER_VERSION}" \
        --set crds.enabled=true \
        --wait

    log_success "Successfully installed cert-manager ${CERT_MANAGER_VERSION} via Helm"

    wait_for_pods "cert-manager" "app in (cert-manager,webhook)" "180s"

    log_success "cert-manager is ready!"
}

if [ "$UNINSTALL" = true ]; then
    uninstall
    exit 0
fi

install
