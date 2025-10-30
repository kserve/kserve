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

# Create KServe Gateway resource
# Usage: manage.kserve-gateway.sh [--reinstall|--uninstall]
#   or:  REINSTALL=true manage.kserve-gateway.sh
#   or:  UNINSTALL=true manage.kserve-gateway.sh

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

# VARIABLES
GATEWAY_NAME="kserve-ingress-gateway"
# GATEWAY_NAMESPACE is defined in global-vars.env (defaults to "kserve")
# Note: For this gateway, GATEWAY_NAMESPACE uses KSERVE_NAMESPACE from global-vars.env
GATEWAY_NAMESPACE="${KSERVE_NAMESPACE}"
GATEWAYCLASS_NAME="envoy"
# VARIABLES END

uninstall() {
    log_info "Deleting KServe Gateway '${GATEWAY_NAME}' in namespace '${GATEWAY_NAMESPACE}'..."
    kubectl delete gateway "${GATEWAY_NAME}" -n "${GATEWAY_NAMESPACE}" --ignore-not-found=true --force --grace-period=0 2>/dev/null || true
    log_success "KServe Gateway '${GATEWAY_NAME}' deleted"
}

install() {
    create_or_skip_namespace "${GATEWAY_NAMESPACE}"

    if kubectl get gateway "${GATEWAY_NAME}" -n "${GATEWAY_NAMESPACE}" &>/dev/null; then
        if [ "$REINSTALL" = false ]; then
            log_info "KServe Gateway '${GATEWAY_NAME}' already exists in namespace '${GATEWAY_NAMESPACE}'. Use --reinstall to recreate."
            exit 0
        else
            log_info "Recreating KServe Gateway '${GATEWAY_NAME}'..."
            uninstall
        fi
    fi

    log_info "Creating KServe Gateway '${GATEWAY_NAME}' in namespace '${GATEWAY_NAMESPACE}'..."
    cat <<EOF | kubectl apply -f -
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: ${GATEWAY_NAME}
  namespace: ${GATEWAY_NAMESPACE}
spec:
  gatewayClassName: ${GATEWAYCLASS_NAME}
  listeners:
    - name: http
      protocol: HTTP
      port: 80
      allowedRoutes:
        namespaces:
          from: All
  infrastructure:
    labels:
      serving.kserve.io/gateway: ${GATEWAY_NAME}
EOF

    log_success "KServe Gateway '${GATEWAY_NAME}' created successfully!"
}

if [ "$UNINSTALL" = true ]; then
    uninstall
    exit 0
fi

install
