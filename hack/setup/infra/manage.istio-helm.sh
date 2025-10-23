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

# Install Istio using Helm
# Usage: manage.istio-helm.sh [--reinstall|--uninstall]
#   or:  REINSTALL=true manage.istio-helm.sh
#   or:  UNINSTALL=true manage.istio-helm.sh
#
# Environment variables for custom Helm values:
#   ISTIO_BASE_EXTRA_ARGS   - Additional helm install arguments for istio-base
#   ISTIOD_EXTRA_ARGS       - Additional helm install arguments for istiod
#   ISTIO_GATEWAY_EXTRA_ARGS - Additional helm install arguments for istio-gateway
#
# Examples:
#   # Minimal resources for dev/test
#   ISTIOD_EXTRA_ARGS="--set resources.requests.cpu=5m --set resources.requests.memory=32Mi"
#   ISTIO_GATEWAY_EXTRA_ARGS="--set resources.requests.cpu=5m --set resources.requests.memory=32Mi --set resources.limits.cpu=100m --set resources.limits.memory=128Mi"
#
#   # Custom configuration
#   ISTIOD_EXTRA_ARGS="--set pilot.traceSampling=100.0 --set meshConfig.enableTracing=true"

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
# ISTIO_NAMESPACE is defined in global-vars.env
# VARIABLES END

uninstall() {
    log_info "Uninstalling Istio..."
    helm uninstall istio-ingressgateway -n "${ISTIO_NAMESPACE}" 2>/dev/null || true
    helm uninstall istiod -n "${ISTIO_NAMESPACE}" 2>/dev/null || true
    helm uninstall istio-base -n "${ISTIO_NAMESPACE}" 2>/dev/null || true
    kubectl delete all --all -n "${ISTIO_NAMESPACE}" --force --grace-period=0 2>/dev/null || true
    kubectl delete namespace "${ISTIO_NAMESPACE}" --wait=true --timeout=60s --force --grace-period=0 2>/dev/null || true
    log_success "Istio uninstalled"
}

install() {
    if helm list -n "${ISTIO_NAMESPACE}" 2>/dev/null | grep -q "istio-base"; then
        if [ "$REINSTALL" = false ]; then
            log_info "Istio is already installed. Use --reinstall to reinstall."
            return 0
        else
            log_info "Reinstalling Istio..."
            uninstall
        fi
    fi

    log_info "Adding Istio Helm repository..."
    helm repo add istio https://istio-release.storage.googleapis.com/charts --force-update

    log_info "Installing istio-base ${ISTIO_VERSION}..."
    helm install istio-base istio/base \
        --namespace "${ISTIO_NAMESPACE}" \
        --create-namespace \
        --version "${ISTIO_VERSION}" \
        --set defaultRevision=default \
        --wait \
        ${ISTIO_BASE_EXTRA_ARGS:-}

    log_info "Installing istiod ${ISTIO_VERSION}..."
    helm install istiod istio/istiod \
        --namespace "${ISTIO_NAMESPACE}" \
        --version "${ISTIO_VERSION}" \
        --set proxy.autoInject=disabled \
        --set-string pilot.podAnnotations."cluster-autoscaler\.kubernetes\.io/safe-to-evict"=true \
        --wait \
        ${ISTIOD_EXTRA_ARGS:-}

    log_info "Installing istio-ingressgateway ${ISTIO_VERSION}..."
    helm install istio-ingressgateway istio/gateway \
        --namespace "${ISTIO_NAMESPACE}" \
        --version "${ISTIO_VERSION}" \
        --set-string podAnnotations."cluster-autoscaler\.kubernetes\.io/safe-to-evict"=true \
        ${ISTIO_GATEWAY_EXTRA_ARGS:-}

    log_success "Successfully installed Istio ${ISTIO_VERSION} via Helm"

    wait_for_pods "${ISTIO_NAMESPACE}" "app=istiod" "600s"
    wait_for_pods "${ISTIO_NAMESPACE}" "app=istio-ingressgateway" "600s"

    log_success "Istio is ready!"
}

if [ "$UNINSTALL" = true ]; then
    uninstall
    exit 0
fi

install
