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

# Create KServe GatewayClass resource
# Usage: manage.gateway-api-gwclass.sh [--reinstall|--uninstall]
#   or:  REINSTALL=true manage.gateway-api-gwclass.sh
#   or:  UNINSTALL=true manage.gateway-api-gwclass.sh

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

# VARIABLES
GATEWAYCLASS_NAME="${GATEWAYCLASS_NAME:-envoy}"
CONTROLLER_NAME="${CONTROLLER_NAME:-gateway.envoyproxy.io/gatewayclass-controller}"
# VARIABLES END

uninstall() {
    log_info "Deleting GatewayClass '${GATEWAYCLASS_NAME}'..."
    kubectl delete gatewayclass "${GATEWAYCLASS_NAME}" --ignore-not-found=true --force --grace-period=0 2>/dev/null || true
    log_success "GatewayClass '${GATEWAYCLASS_NAME}' deleted"
}

install() {
    if kubectl get gatewayclass "${GATEWAYCLASS_NAME}" &>/dev/null; then
        if [ "$REINSTALL" = false ]; then
            log_info "GatewayClass '${GATEWAYCLASS_NAME}' already exists. Use --reinstall to recreate."
            return 0
        else
            log_info "Recreating GatewayClass '${GATEWAYCLASS_NAME}'..."
            uninstall
        fi
    fi

    log_info "Creating GatewayClass '${GATEWAYCLASS_NAME}'..."
    cat <<EOF | kubectl apply -f -
apiVersion: gateway.networking.k8s.io/v1
kind: GatewayClass
metadata:
  name: ${GATEWAYCLASS_NAME}
spec:
  controllerName: ${CONTROLLER_NAME}
EOF

    log_success "GatewayClass '${GATEWAYCLASS_NAME}' created successfully!"
}

if [ "$UNINSTALL" = true ]; then
    uninstall
    exit 0
fi

install
