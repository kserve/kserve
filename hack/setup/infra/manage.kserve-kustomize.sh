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

# Install KServe using Kustomize (from local config/default)
# Usage: manage.kserve-kustomize.sh [--reinstall|--uninstall]
#   or:  REINSTALL=true manage.kserve-kustomize.sh
#   or:  UNINSTALL=true manage.kserve-kustomize.sh
#
# Environment variables:
#   LLMISVC - Enable LLM Inference Service Controller (default: false)
#
#   DEPLOYMENT_MODE - Default deployment mode
#                     Supported values:
#                       - Serverless (legacy, converted to Knative)
#                       - Knative (serverless deployment with Knative)
#                       - RawDeployment (legacy, converted to Standard)
#                       - Standard (standard Kubernetes deployment)
#                     Default: Knative
#
#   GATEWAY_NETWORK_LAYER - false, envoy, istio (default: false)
#                           if it is not false, ENABLE_GATEWAY_API will be set true   
#
# This script installs KServe directly from the local config directories
# using kubectl kustomize for development and testing.
#
# Examples:
#   # Use default config with Serverless/Knative mode
#   ./manage.kserve-kustomize.sh
#
#   # Use Standard deployment mode (raw Kubernetes)
#   DEPLOYMENT_MODE=Standard ./manage.kserve-kustomize.sh
#
#   # Enable LLM Inference Service Controller
#   LLSMISVC=true ./manage.kserve-kustomize.sh

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

check_cli_exist kubectl

# VARIABLES
# KSERVE_NAMESPACE is defined in global-vars.env
KSERVE_CRD_DIRS=(
    "${REPO_ROOT}/config/crd/full"
    "${REPO_ROOT}/config/crd/full/llmisvc"
    "${REPO_ROOT}/config/crd/full/localmodel"
)
KSERVE_CONFIG_DIR="${REPO_ROOT}/config/overlays/all"
KSERVE_OVERYLAY_DIR="${KSERVE_OVERYLAY_DIR:-}"
TARGET_DEPLOYMENT_NAMES=(
    "kserve-controller-manager"
    "kserve-localmodel-controller-manager"
    "llmisvc-controller-manager"
)
# DEPLOYMENT_MODE, GATEWAY_NETWORK_LAYER, LLMISVC, EMBED_MANIFESTS are defined in global-vars.env
INSTALL_RUNTIMES="${INSTALL_RUNTIMES:-false}"
# VARIABLES END

# INCLUDE_IN_GENERATED_SCRIPT_START
# Set CRD/Config directories and target pod labels based on LLMISVC
if [ "${KSERVE_OVERYLAY_DIR}" != "" ]; then
    KSERVE_CONFIG_DIR="${REPO_ROOT}/config/overlays/${KSERVE_OVERYLAY_DIR}"
fi

if [ "${LLMISVC}" = "true" ]; then
    KSERVE_CRD_DIRS=(
        "${REPO_ROOT}/config/crd/full/llmisvc"
    )
    KSERVE_CONFIG_DIR="${REPO_ROOT}/config/overlays/standalone/llmisvc"
    TARGET_DEPLOYMENT_NAMES=("llmisvc-controller-manager")
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
        # Development mode: use kustomize
        # Uninstall resources first
        kubectl kustomize "${KSERVE_CONFIG_DIR}" | kubectl delete -f - --force --grace-period=0 2>/dev/null || true

        # Then uninstall CRDs
        for crd_dir in "${KSERVE_CRD_DIRS[@]}"; do
            kubectl kustomize "${crd_dir}" | kubectl delete -f - --force --grace-period=0 2>/dev/null || true
        done
    fi

    kubectl delete all --all -n "${KSERVE_NAMESPACE}" --force --grace-period=0 2>/dev/null || true
    kubectl delete namespace "${KSERVE_NAMESPACE}" --wait=true --timeout=60s --force --grace-period=0 2>/dev/null || true
    log_success "KServe uninstalled"
}

install() {
    if kubectl get deployment kserve-controller-manager -n "${KSERVE_NAMESPACE}" &>/dev/null; then
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
        log_info "Installing KServe using embedded manifests..."

        # Call manifest functions (these should be available in generated script)
        if type install_kserve_manifest &>/dev/null; then
            install_kserve_manifest
        else
            log_error "EMBED_MANIFESTS enabled but install_kserve_manifest function not found"
            log_error "This script should be called from a generated installation script"
            exit 1
        fi
    else
        # Development mode: use local kustomize build
        log_info "Installing KServe via Kustomize..."
        log_info "📍 Using local config from ${KSERVE_CRD_DIRS[*]} and ${KSERVE_CONFIG_DIR}"

        # Install CRDs first
        log_info "Installing KServe CRDs..."
        for crd_dir in "${KSERVE_CRD_DIRS[@]}"; do
            log_info "  - Installing CRDs from ${crd_dir}..."
            kustomize build "${crd_dir}" | kubectl apply --server-side --force-conflicts -f -
        done

        # Wait for CRDs to be established       
        if [ "${LLMISVC}" = "true" ]; then
            wait_for_crds "60s" \
                "llminferenceservices.serving.kserve.io" \
                "llminferenceserviceconfigs.serving.kserve.io"
        else
            wait_for_crds "60s" \
                "inferenceservices.serving.kserve.io" \
                "servingruntimes.serving.kserve.io" \
                "clusterservingruntimes.serving.kserve.io" \
                "llminferenceservices.serving.kserve.io" \
                "llminferenceserviceconfigs.serving.kserve.io"            
        fi
        # Install resources
        log_info "Installing KServe resources..."
        kustomize build "${KSERVE_CONFIG_DIR}" | kubectl apply --server-side -f -
    fi

    # Build list of config updates
    local config_updates=()

    # Update deployment mode if needed
    if [ "${DEPLOYMENT_MODE}" = "Standard" ] || [ "${DEPLOYMENT_MODE}" = "RawDeployment" ]; then
        log_info "Adding deployment mode update: ${DEPLOYMENT_MODE}"
        config_updates+=("deploy.defaultDeploymentMode=\"${DEPLOYMENT_MODE}\"")
    fi

    # Enable Gateway API for KServe(ISVC) if needed
    if [ "${GATEWAY_NETWORK_LAYER}" != "false" ] && [ "${LLMISVC}" != "true" ]; then
        log_info "Adding Gateway API updates: enableGatewayApi=true, ingressClassName=${GATEWAY_NETWORK_LAYER}"
        config_updates+=("ingress.enableGatewayApi=true")
        config_updates+=("ingress.ingressClassName=\"${GATEWAY_NETWORK_LAYER}\"")
    fi

    # Add custom configurations if provided
    if [ -n "${KSERVE_CUSTOM_ISVC_CONFIGS}" ]; then
        log_info "Adding custom configurations: ${KSERVE_CUSTOM_ISVC_CONFIGS}"
        IFS='|' read -ra custom_configs <<< "${KSERVE_CUSTOM_ISVC_CONFIGS}"
        config_updates+=("${custom_configs[@]}")
    fi

    # Apply all config updates at once if there are any
    if [ ${#config_updates[@]} -gt 0 ]; then
        log_info "Applying ${#config_updates[@]} configuration update(s):"
        for update in "${config_updates[@]}"; do
            log_info "  - ${update}"
        done
        update_isvc_config "${config_updates[@]}"
        if [ "${LLMISVC}" != "true" ]; then
            kubectl rollout restart deployment kserve-controller-manager -n ${KSERVE_NAMESPACE}
        fi
    else
        if [ "${LLMISVC}" = "true" ]; then
            log_info "No configuration updates needed for LLMISVC (GATEWAY_NETWORK_LAYER=${GATEWAY_NETWORK_LAYER})"
        else
            log_info "No configuration updates needed (DEPLOYMENT_MODE=${DEPLOYMENT_MODE}, GATEWAY_NETWORK_LAYER=${GATEWAY_NETWORK_LAYER})"
        fi
    fi

    log_success "Successfully installed KServe"

    # Wait for all controller managers to be ready
    log_info "Waiting for KServe controllers to be ready..."
    for deploy in "${TARGET_DEPLOYMENT_NAMES[@]}"; do
        wait_for_deployment "${KSERVE_NAMESPACE}" "${deploy}" "300s"
    done

    log_success "KServe is ready!"
    if [ ${INSTALL_RUNTIMES} = "true" ]; then
        kubectl apply --server-side=true -k config/runtimes
    fi
}

if [ "$UNINSTALL" = true ]; then
    uninstall
    exit 0
fi

install
