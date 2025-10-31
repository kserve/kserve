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

# Install Envoy Gateway using Helm
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
    log_info "Uninstalling Envoy Gateway..."
    kubectl delete gatewayclass envoy --ignore-not-found=true --force --grace-period=0 2>/dev/null || true
    helm uninstall eg -n envoy-gateway-system 2>/dev/null || true
    kubectl delete all --all -n envoy-gateway-system --force --grace-period=0 2>/dev/null || true
    kubectl delete namespace envoy-gateway-system --wait=true --timeout=60s --force --grace-period=0 2>/dev/null || true
    log_success "Envoy Gateway uninstalled"
}

install() {
    if helm list -n envoy-gateway-system 2>/dev/null | grep -q "eg"; then
        if [ "$REINSTALL" = false ]; then
            log_info "Envoy Gateway is already installed. Use --reinstall to reinstall."
            return 0
        else
            log_info "Reinstalling Envoy Gateway..."
            uninstall
        fi
    fi

    log_info "Installing Envoy Gateway ${ENVOY_GATEWAY_VERSION}..."
    helm install eg oci://docker.io/envoyproxy/gateway-helm \
        --version "${ENVOY_GATEWAY_VERSION}" \
        -n envoy-gateway-system \
        --create-namespace \
        --wait

    log_success "Successfully installed Envoy Gateway ${ENVOY_GATEWAY_VERSION} via Helm"

    wait_for_pods "envoy-gateway-system" "control-plane=envoy-gateway" "300s"

    log_info "Creating Envoy GatewayClass..."
    cat <<EOF | kubectl apply -f -
apiVersion: gateway.networking.k8s.io/v1
kind: GatewayClass
metadata:
  name: envoy
spec:
  controllerName: gateway.envoyproxy.io/gatewayclass-controller
EOF

    log_success "Envoy Gateway is ready!"
}

if [ "$UNINSTALL" = true ]; then
    uninstall
    exit 0
fi

install
