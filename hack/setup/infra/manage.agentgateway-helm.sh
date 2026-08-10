#!/bin/bash

# Copyright 2026 The KServe Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Install agentgateway using Helm with Gateway API Inference Extension support.
# Usage: manage.agentgateway-helm.sh [--reinstall|--uninstall]
#   or:  REINSTALL=true manage.agentgateway-helm.sh
#   or:  UNINSTALL=true manage.agentgateway-helm.sh

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

check_cli_exist helm kubectl

NAMESPACE="agentgateway-system"
RELEASE_NAME="agentgateway"
CRD_RELEASE_NAME="agentgateway-crds"

uninstall() {
    log_info "Uninstalling agentgateway..."
    helm uninstall "${RELEASE_NAME}" -n "${NAMESPACE}" 2>/dev/null || true
    helm uninstall "${CRD_RELEASE_NAME}" -n "${NAMESPACE}" 2>/dev/null || true
    kubectl delete namespace "${NAMESPACE}" --ignore-not-found=true --wait=true --timeout=60s 2>/dev/null || true
    log_success "agentgateway uninstalled"
}

install() {
    if helm list -n "${NAMESPACE}" 2>/dev/null | grep -q "^${RELEASE_NAME}[[:space:]]"; then
        if [[ "${REINSTALL}" == "false" ]]; then
            log_info "agentgateway is already installed. Use --reinstall to reinstall."
            return 0
        fi
        log_info "Reinstalling agentgateway..."
        uninstall
    fi

    log_info "Installing agentgateway CRDs ${AGENTGATEWAY_VERSION}..."
    helm upgrade -i "${CRD_RELEASE_NAME}" \
        oci://cr.agentgateway.dev/charts/agentgateway-crds \
        --version "${AGENTGATEWAY_VERSION}" \
        -n "${NAMESPACE}" \
        --create-namespace \
        --wait

    log_info "Installing agentgateway ${AGENTGATEWAY_VERSION} with Inference Extension support..."
    helm upgrade -i "${RELEASE_NAME}" \
        oci://cr.agentgateway.dev/charts/agentgateway \
        --version "${AGENTGATEWAY_VERSION}" \
        -n "${NAMESPACE}" \
        --create-namespace \
        --set inferenceExtension.enabled=true \
        --wait

    kubectl rollout status deployment/agentgateway -n "${NAMESPACE}" --timeout=300s
    kubectl wait gatewayclass/agentgateway --for=condition=Accepted --timeout=120s

    log_success "agentgateway is ready!"
}

if [[ "${UNINSTALL}" == "true" ]]; then
    uninstall
    exit 0
fi

install
