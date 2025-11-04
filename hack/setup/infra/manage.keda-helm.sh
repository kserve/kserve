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

# Install KEDA using Helm
# Usage: manage.keda-helm.sh [--reinstall|--uninstall]
#   or:  REINSTALL=true manage.keda-helm.sh
#   or:  UNINSTALL=true manage.keda-helm.sh
#
# Environment variables for custom Helm values:
#   KEDA_EXTRA_ARGS - Additional helm install arguments for KEDA
#
# Examples:
#   KEDA_EXTRA_ARGS="--set resources.operator.limits.cpu=1000m"

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
# VARIABLES END

uninstall() {
    log_info "Uninstalling KEDA..."

    helm uninstall keda-otel-scaler -n "${KEDA_NAMESPACE}" 2>/dev/null || true
    helm uninstall keda -n "${KEDA_NAMESPACE}" 2>/dev/null || true
    kubectl delete all --all -n "${KEDA_NAMESPACE}" --force --grace-period=0 2>/dev/null || true
    kubectl delete namespace "${KEDA_NAMESPACE}" --wait=true --timeout=60s --force --grace-period=0 2>/dev/null || true

    log_success "KEDA uninstalled"
}

install() {
    if helm list -n "${KEDA_NAMESPACE}" 2>/dev/null | grep -q "keda"; then
        if [ "$REINSTALL" = false ]; then
            log_info "KEDA is already installed. Use --reinstall to reinstall."
            return 0
        else
            log_info "Reinstalling KEDA..."
            uninstall
        fi
    fi

    log_info "Adding KEDA Helm repository..."
    helm repo add kedacore https://kedacore.github.io/charts --force-update

    log_info "Installing KEDA ${KEDA_VERSION}..."
    helm install keda kedacore/keda \
        --namespace "${KEDA_NAMESPACE}" \
        --create-namespace \
        --version "${KEDA_VERSION}" \
        --wait \
        ${KEDA_EXTRA_ARGS:-}

    log_success "Successfully installed KEDA ${KEDA_VERSION} via Helm"

    wait_for_pods "${KEDA_NAMESPACE}" "app.kubernetes.io/name=keda-operator" "300s"

    log_success "KEDA is ready!"
}

if [ "$UNINSTALL" = true ]; then
    uninstall
    exit 0
fi

install
