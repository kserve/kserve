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

# Install Knative Operator using Helm and deploy Knative Serving
# Usage: manage.knative-operator-helm.sh [--network-layer=istio|kourier]
#   or:  NETWORK_LAYER=kourier manage.knative-operator-helm.sh
#
# Environment variables:
#   NETWORK_LAYER    - Network layer: istio or kourier (default: istio)
#
# Note: Versions are managed in kserve-deps.env
#
# Examples:
#   # Install with Istio network layer (default)
#   ./manage.knative-operator-helm.sh
#
#   # Install with Kourier network layer
#   NETWORK_LAYER=kourier ./manage.knative-operator-helm.sh

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
# OPERATOR_NAMESPACE and SERVING_NAMESPACE are defined in global-vars.env
NETWORK_LAYER="${NETWORK_LAYER:-istio}"
TEMPLATE_DIR="${SCRIPT_DIR}/templates"
# VARIABLES END

check_cli_exist helm

# Parse network layer from arguments
if [[ "$*" == *"--network-layer="* ]]; then
    NETWORK_LAYER=$(echo "$*" | sed -n 's/.*--network-layer=\([^ ]*\).*/\1/p')
fi

# Validate network layer
if [[ "${NETWORK_LAYER}" != "istio" && "${NETWORK_LAYER}" != "kourier" ]]; then
    log_error "Invalid network layer: ${NETWORK_LAYER}. Must be 'istio' or 'kourier'"
    exit 1
fi

uninstall() {
    log_info "Uninstalling Knative Serving..."
    kubectl delete -f "${TEMPLATE_DIR}/knative-serving-${NETWORK_LAYER}.yaml" --ignore-not-found=true --force --grace-period=0 2>/dev/null || true
    kubectl delete all --all -n "${SERVING_NAMESPACE}" --force --grace-period=0 2>/dev/null || true
    kubectl delete namespace "${SERVING_NAMESPACE}" --wait=true --timeout=60s --force --grace-period=0 2>/dev/null || true

    log_info "Uninstalling Knative Operator..."
    helm uninstall knative-operator -n "${OPERATOR_NAMESPACE}" 2>/dev/null || true
    kubectl delete all --all -n "${OPERATOR_NAMESPACE}" --force --grace-period=0 2>/dev/null || true
    kubectl delete namespace "${OPERATOR_NAMESPACE}" --wait=true --timeout=60s --force --grace-period=0 2>/dev/null || true

    log_success "Knative uninstalled"
}

install() {
    log_info "Network layer: ${NETWORK_LAYER}"

    if helm list -n "${OPERATOR_NAMESPACE}" 2>/dev/null | grep -q "knative-operator"; then
        if [ "$REINSTALL" = false ]; then
            log_info "Knative Operator is already installed. Checking Knative Serving..."

            if kubectl get knativeserving knative-serving -n "${SERVING_NAMESPACE}" &>/dev/null; then
                log_info "Knative Serving is already deployed. Use --reinstall to reinstall."
                exit 0
            fi
        else
            log_info "Reinstalling Knative..."
            uninstall
        fi
    fi

    log_info "Installing Knative Operator ${KNATIVE_OPERATOR_VERSION}..."

    if [[ "${KNATIVE_OPERATOR_VERSION}" == v* ]]; then
        OPERATOR_CHART_URL="https://github.com/knative/operator/releases/download/knative-${KNATIVE_OPERATOR_VERSION}/knative-operator-${KNATIVE_OPERATOR_VERSION}.tgz"
        log_info "Using GitHub release: ${OPERATOR_CHART_URL}"

        # shellcheck disable=SC2086
        helm install knative-operator \
            --namespace "${OPERATOR_NAMESPACE}" \
            --create-namespace \
            --wait \
            ${KNATIVE_OPERATOR_EXTRA_ARGS:-} \
            "${OPERATOR_CHART_URL}"
    else
        log_info "Adding Knative Operator Helm repository..."
        helm repo add knative-operator https://knative.github.io/operator --force-update

        # shellcheck disable=SC2086
        helm install knative-operator knative-operator/knative-operator \
            --namespace "${OPERATOR_NAMESPACE}" \
            --create-namespace \
            --version "${KNATIVE_OPERATOR_VERSION}" \
            --wait \
            ${KNATIVE_OPERATOR_EXTRA_ARGS:-}
    fi

    log_success "Successfully installed Knative Operator ${KNATIVE_OPERATOR_VERSION}"

    wait_for_pods "${OPERATOR_NAMESPACE}" "name=knative-operator" "300s"

    log_info "Deploying Knative Serving ${KNATIVE_SERVING_VERSION} with ${NETWORK_LAYER} network layer..."

    TEMPLATE_FILE="${TEMPLATE_DIR}/knative-serving-${NETWORK_LAYER}.yaml"

    if [[ ! -f "${TEMPLATE_FILE}" ]]; then
        log_error "Template file not found: ${TEMPLATE_FILE}"
        exit 1
    fi

    if [[ "${KNATIVE_SERVING_VERSION}" != "1.15.2" ]]; then
        log_info "Customizing template with version=${KNATIVE_SERVING_VERSION}"
        sed -e "s/version: \".*\"/version: \"${KNATIVE_SERVING_VERSION}\"/" \
            "${TEMPLATE_FILE}" | kubectl apply -f -
    else
        kubectl apply -f "${TEMPLATE_FILE}"
    fi

    log_success "Knative Serving CR applied"

    log_info "Waiting for Knative Serving to be ready..."
    kubectl wait --for=condition=Ready -n "${SERVING_NAMESPACE}" KnativeServing knative-serving --timeout=300s

    log_success "Knative Operator and Serving are ready!"
}

if [ "$UNINSTALL" = true ]; then
    uninstall
    exit 0
fi

install
