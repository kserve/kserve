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

# Install Jaeger All-in-One using Helm (for tracing e2e tests)
# Usage: manage.jaeger-helm.sh [--reinstall|--uninstall]
#   or:  REINSTALL=true manage.jaeger-helm.sh
#   or:  UNINSTALL=true manage.jaeger-helm.sh
#
# Environment variables for custom Helm values:
#   JAEGER_EXTRA_ARGS   - Additional helm install arguments for Jaeger

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
JAEGER_NAMESPACE="${JAEGER_NAMESPACE:-observability}"
JAEGER_RELEASE_NAME="${JAEGER_RELEASE_NAME:-jaeger}"
# VARIABLES END

uninstall() {
    log_info "Uninstalling Jaeger..."
    helm uninstall "${JAEGER_RELEASE_NAME}" -n "${JAEGER_NAMESPACE}" 2>/dev/null || true
    kubectl delete all --all -n "${JAEGER_NAMESPACE}" --force --grace-period=0 2>/dev/null || true
    kubectl delete namespace "${JAEGER_NAMESPACE}" --wait=true --timeout=60s --force --grace-period=0 2>/dev/null || true
    log_success "Jaeger uninstalled"
}

install() {
    if helm list -n "${JAEGER_NAMESPACE}" 2>/dev/null | grep -q "${JAEGER_RELEASE_NAME}"; then
        if [ "$REINSTALL" = false ]; then
            log_info "Jaeger is already installed. Use --reinstall to reinstall."
            return 0
        else
            log_info "Reinstalling Jaeger..."
            uninstall
        fi
    fi

    log_info "Adding Jaeger Helm repository..."
    helm repo add jaegertracing https://jaegertracing.github.io/helm-charts --force-update

    log_info "Installing Jaeger All-in-One ${JAEGER_VERSION}..."
    helm install "${JAEGER_RELEASE_NAME}" jaegertracing/jaeger \
        --namespace "${JAEGER_NAMESPACE}" \
        --create-namespace \
        --version "${JAEGER_VERSION}" \
        --wait \
        --timeout 5m \
        ${JAEGER_EXTRA_ARGS:-}

    log_success "Successfully installed Jaeger All-in-One via Helm"

    wait_for_pods "${JAEGER_NAMESPACE}" "app.kubernetes.io/name=jaeger" "300s"

    log_success "Jaeger is ready!"
}

if [ "$UNINSTALL" = true ]; then
    uninstall
    exit 0
fi

install
