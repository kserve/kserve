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

# Install KServe using Helm
# Usage: manage.kserve-helm.sh [--reinstall|--uninstall]
#   or:  REINSTALL=true manage.kserve-helm.sh
#   or:  UNINSTALL=true manage.kserve-helm.sh
#
# Environment variables:
#   USE_LOCAL_CHARTS      - Use local charts instead of OCI registry (default: false)
#   KSERVE_VERSION        - KServe version to install (default: from kserve-deps.env)
#   KSERVE_CRD_EXTRA_ARGS - Additional helm install arguments for KServe CRDs
#   KSERVE_EXTRA_ARGS     - Additional helm install arguments for KServe resources
#
# Examples:
#   # Install from OCI registry (uses version from kserve-deps.env)
#   ./manage.kserve-helm.sh
#
#   # Install specific version from OCI registry
#   KSERVE_VERSION=v0.15.0 ./manage.kserve-helm.sh
#
#   # Install from local charts (development)
#   USE_LOCAL_CHARTS=true ./manage.kserve-helm.sh
#
#   # Custom resource limits
#   KSERVE_EXTRA_ARGS="--set kserve.controller.resources.limits.cpu=500m" ./manage.kserve-helm.sh
#
#   # Custom controller image for local development
#   USE_LOCAL_CHARTS=true KSERVE_EXTRA_ARGS="--set kserve.controller.tag=local-test --set kserve.controller.imagePullPolicy=Never" ./manage.kserve-helm.sh


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

# VARIABLES
# KSERVE_NAMESPACE is defined in global-vars.env
KSERVE_CRD_RELEASE_NAME="kserve-crd"
KSERVE_RELEASE_NAME="kserve"
CRD_DIR_NAME="kserve-crd"
CORE_DIR_NAME="kserve-resources"
TARGET_POD_LABELS=(
    "control-plane=kserve-controller-manager"
    "app.kubernetes.io/name=kserve-localmodel-controller-manager"
    "app.kubernetes.io/name=llmisvc-controller-manager"
)
DEPLOYMENT_MODE="${DEPLOYMENT_MODE:-Knative}"
USE_LOCAL_CHARTS="${USE_LOCAL_CHARTS:-false}"
LLMISVC="${LLMISVC:-false}"
CHARTS_DIR="${REPO_ROOT}/charts"
EMBED_MANIFESTS="${EMBED_MANIFESTS:-false}"
# VARIABLES END

# INCLUDE_IN_GENERATED_SCRIPT_START
# Set Helm release names and target pod labels based on LLMISVC
if [ "${LLMISVC}" = "true" ]; then
    CRD_DIR_NAME="llmisvc-crd"
    CORE_DIR_NAME="llmisvc-resources"
    KSERVE_CRD_RELEASE_NAME="llmisvc-crd"
    KSERVE_RELEASE_NAME="llmisvc"
    TARGET_POD_LABELS=("control-plane=llmisvc-controller-manager")
fi
# INCLUDE_IN_GENERATED_SCRIPT_END

uninstall() {
    log_info "Uninstalling KServe..."

    # EMBED_MANIFESTS: use embedded manifests
    if [ "$EMBED_MANIFESTS" = "true" ]; then
        if type uninstall_kserve_manifest &>/dev/null; then
            uninstall_kserve_manifest
        else
            log_error "EMBED_MANIFESTS enabled but uninstall_kserve_manifest function not found"
            log_error "This script should be called from a generated installation script"
            exit 1
        fi
    else
        # Development/Helm mode
        helm uninstall "${KSERVE_RELEASE_NAME}" -n "${KSERVE_NAMESPACE}" 2>/dev/null || true
        helm uninstall "${KSERVE_CRD_RELEASE_NAME}" -n "${KSERVE_NAMESPACE}" 2>/dev/null || true
    fi

    kubectl delete all --all -n "${KSERVE_NAMESPACE}" --force --grace-period=0 2>/dev/null || true
    kubectl delete namespace "${KSERVE_NAMESPACE}" --wait=true --timeout=60s --force --grace-period=0 2>/dev/null || true
    log_success "KServe uninstalled"
}

install() {
    if helm list -n "${KSERVE_NAMESPACE}" 2>/dev/null | grep -q "${KSERVE_RELEASE_NAME}"; then
        if [ "$REINSTALL" = false ]; then
            log_info "KServe is already installed. Use --reinstall to reinstall."
            return 0
        else
            log_info "Reinstalling KServe..."
            uninstall
        fi
    fi
    
    # EMBED_MANIFESTS: use embedded manifests from generated script
    if [ "$EMBED_MANIFESTS" = "true" ]; then
        log_info "Installing KServe using embedded manifests ..."

        # Call manifest functions (these should be available in generated script)
        if type install_kserve_manifest &>/dev/null; then
            install_kserve_manifest
        else
            log_error "EMBED_MANIFESTS enabled but install_kserve_manifest function not found"
            log_error "This script should be called from a generated installation script"
            exit 1
        fi
    elif [ "${USE_LOCAL_CHARTS}" = true ]; then
        # Install KServe using local charts (for development)
        log_info "Installing KServe using local charts..."
        log_info "📍 Using local charts from ${CHARTS_DIR}/"

        # Install KServe CRDs from local chart
        log_info "Installing KServe CRDs..."
        helm install "${KSERVE_CRD_RELEASE_NAME}" "${CHARTS_DIR}/${CRD_DIR_NAME}" \
            --namespace "${KSERVE_NAMESPACE}" \
            --create-namespace \
            --wait \
            ${KSERVE_CRD_EXTRA_ARGS:-}

        # Install KServe resources from local chart
        log_info "Installing KServe resources..."
        helm install "${KSERVE_RELEASE_NAME}" "${CHARTS_DIR}/${CORE_DIR_NAME}" \
            --namespace "${KSERVE_NAMESPACE}" \
            --create-namespace \
            --wait \
            ${KSERVE_EXTRA_ARGS:-}

        log_success "Successfully installed KServe using local charts"
    else
        # Install KServe from OCI registry
        log_info "Installing KServe ${KSERVE_VERSION} from OCI registry..."

        # Install KServe CRDs
        log_info "Installing KServe CRDs..."
        helm install "${KSERVE_CRD_RELEASE_NAME}" \
            oci://ghcr.io/kserve/charts/${CRD_DIR_NAME} \
            --version "${KSERVE_VERSION}" \
            --namespace "${KSERVE_NAMESPACE}" \
            --create-namespace \
            --wait \
            ${KSERVE_CRD_EXTRA_ARGS:-}

        # Install KServe resources
        log_info "Installing KServe resources..."
        helm install "${KSERVE_RELEASE_NAME}" \
            oci://ghcr.io/kserve/charts/${CORE_DIR_NAME} \
            --version "${KSERVE_VERSION}" \
            --namespace "${KSERVE_NAMESPACE}" \
            --create-namespace \
            --wait \
            ${KSERVE_EXTRA_ARGS:-}

        log_success "Successfully installed KServe ${KSERVE_VERSION}"
    fi

    # Update deployment mode in ConfigMap if not default
    if [ "${DEPLOYMENT_MODE}" != "Knative" ]; then
        log_info "Configuring deployment mode: ${DEPLOYMENT_MODE}"
        kubectl patch configmap inferenceservice-config -n "${KSERVE_NAMESPACE}" \
            --type='merge' \
            -p "{\"data\":{\"deploy\":\"{\\\"defaultDeploymentMode\\\":\\\"${DEPLOYMENT_MODE}\\\"}\" }}"
    fi

    for label in "${TARGET_POD_LABELS[@]}"; do
        wait_for_pods "${KSERVE_NAMESPACE}" "${label}" "300s"
    done
    log_success "KServe is ready!"
}

if [ "$UNINSTALL" = true ]; then
    uninstall
    exit 0
fi

install
