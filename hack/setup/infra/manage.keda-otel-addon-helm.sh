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

# Install KEDA OTel add-on using Helm
# Usage: manage.keda-otel-addon-helm.sh [--reinstall|--uninstall]
#   or:  REINSTALL=true manage.keda-otel-addon-helm.sh
#   or:  UNINSTALL=true manage.keda-otel-addon-helm.sh
#
# Environment variables for custom Helm values:
#   KEDA_OTEL_ADDON_EXTRA_ARGS - Additional helm install arguments
#
# Example:
#   KEDA_OTEL_ADDON_EXTRA_ARGS="--set validatingAdmissionPolicy.enabled=false"

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

# VARIABLES
# KEDA_NAMESPACE is defined in global-vars.env
ADDON_RELEASE_NAME="keda-otel-scaler"
# VARIABLES END

uninstall() {
    log_info "Uninstalling KEDA OTel add-on..."
    helm uninstall "${ADDON_RELEASE_NAME}" -n "${KEDA_NAMESPACE}" 2>/dev/null || true
    log_success "KEDA OTel add-on uninstalled"
}

install() {
    if ! kubectl get namespace "${KEDA_NAMESPACE}" &>/dev/null; then
        log_error "KEDA namespace '${KEDA_NAMESPACE}' does not exist. Please install KEDA first."
        exit 1
    fi

    if helm list -n "${KEDA_NAMESPACE}" 2>/dev/null | grep -q "${ADDON_RELEASE_NAME}"; then
        if [ "$REINSTALL" = false ]; then
            log_info "KEDA OTel add-on is already installed. Use --reinstall to reinstall."
            return 0
        else
            log_info "Reinstalling KEDA OTel add-on..."
            uninstall
        fi
    fi

    log_info "Installing KEDA OTel add-on ${KEDA_OTEL_ADDON_VERSION} from kedify/otel-add-on..."
    helm upgrade -i "${ADDON_RELEASE_NAME}" \
        oci://ghcr.io/kedify/charts/otel-add-on \
        --namespace "${KEDA_NAMESPACE}" \
        --version="${KEDA_OTEL_ADDON_VERSION}" \
        --wait \
        ${KEDA_OTEL_ADDON_EXTRA_ARGS:-}

    log_success "Successfully installed KEDA OTel add-on ${KEDA_OTEL_ADDON_VERSION} via Helm"

    wait_for_pods "${KEDA_NAMESPACE}" "app.kubernetes.io/instance=${ADDON_RELEASE_NAME}" "300s"

    log_success "KEDA OTel add-on is ready!"
}

if [ "$UNINSTALL" = true ]; then
    uninstall
    exit 0
fi

install
