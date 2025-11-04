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

# Install OpenTelemetry Operator using Helm
# Usage: manage.opentelemetry-operator-helm.sh [--reinstall|--uninstall]
#   or:  REINSTALL=true manage.opentelemetry-operator-helm.sh
#   or:  UNINSTALL=true manage.opentelemetry-operator-helm.sh
#
# Environment variables for custom Helm values:
#   OTEL_OPERATOR_EXTRA_ARGS   - Additional helm install arguments for OpenTelemetry Operator
#
# Examples:
#   OTEL_OPERATOR_EXTRA_ARGS="--set manager.collectorImage.repository=otel/opentelemetry-collector-contrib --set manager.resources.limits.cpu=500m"

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
# OTEL_NAMESPACE is defined in global-vars.env
OTEL_RELEASE_NAME="my-opentelemetry-operator"
# VARIABLES END

uninstall() {
    log_info "Uninstalling OpenTelemetry Operator..."
    helm uninstall "${OTEL_RELEASE_NAME}" -n "${OTEL_NAMESPACE}" 2>/dev/null || true
    kubectl delete all --all -n "${OTEL_NAMESPACE}" --force --grace-period=0 2>/dev/null || true
    kubectl delete namespace "${OTEL_NAMESPACE}" --wait=true --timeout=60s --force --grace-period=0 2>/dev/null || true
    log_success "OpenTelemetry Operator uninstalled"
}

install() {
    if helm list -n "${OTEL_NAMESPACE}" 2>/dev/null | grep -q "${OTEL_RELEASE_NAME}"; then
        if [ "$REINSTALL" = false ]; then
            log_info "OpenTelemetry Operator is already installed. Use --reinstall to reinstall."
            return 0
        else
            log_info "Reinstalling OpenTelemetry Operator..."
            uninstall
        fi
    fi

    log_info "Adding OpenTelemetry Helm repository..."
    helm repo add open-telemetry https://open-telemetry.github.io/opentelemetry-helm-charts --force-update

    log_info "Installing OpenTelemetry Operator..."
    helm install "${OTEL_RELEASE_NAME}" open-telemetry/opentelemetry-operator \
        --namespace "${OTEL_NAMESPACE}" \
        --create-namespace \
        --wait \
        ${OTEL_OPERATOR_EXTRA_ARGS:-}

    log_success "Successfully installed OpenTelemetry Operator via Helm"

    wait_for_pods "${OTEL_NAMESPACE}" "app.kubernetes.io/instance=${OTEL_RELEASE_NAME}" "300s"

    log_success "OpenTelemetry Operator is ready!"
}

if [ "$UNINSTALL" = true ]; then
    uninstall
    exit 0
fi

install
