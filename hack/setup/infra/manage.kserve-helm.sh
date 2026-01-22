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
#   SET_KSERVE_VERSION    - KServe version to install (default: from kserve-deps.env)
#   KSERVE_CRD_EXTRA_ARGS - Additional helm install arguments for KServe CRDs
#   KSERVE_EXTRA_ARGS     - Additional helm install arguments for KServe resources
#
#   LLMISVC - Enable LLM Inference Service Controller (default: false)
#   DEPLOYMENT_MODE - Default deployment mode
#                     Supported values:
#                       - Serverless (legacy, converted to Knative)
#                       - Knative (serverless deployment with Knative)
#                       - RawDeployment (legacy, converted to Standard)
#                       - Standard (standard Kubernetes deployment)
#                     Default: Knative
#
#   GATEWAY_NETWORK_LAYER - false, envoy, istio (default: false)
#                           if it is not false, enableGatewayApi will be set true
#
# Examples:
#   # Install from OCI registry (uses version from kserve-deps.env)
#   ./manage.kserve-helm.sh
#
#   # Install specific version from OCI registry
#   SET_KSERVE_VERSION=v0.15.0 ./manage.kserve-helm.sh
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
TARGET_DEPLOYMENT_NAMES=(
    "kserve-controller-manager"
)
# DEPLOYMENT_MODE, GATEWAY_NETWORK_LAYER, LLMISVC, EMBED_MANIFESTS are defined in global-vars.env
USE_LOCAL_CHARTS="${USE_LOCAL_CHARTS:-false}"
CHARTS_DIR="${REPO_ROOT}/charts"
SET_KSERVE_VERSION="${SET_KSERVE_VERSION:-}"
# VARIABLES END

# INCLUDE_IN_GENERATED_SCRIPT_START
# Set Helm release names and target pod labels based on LLMISVC
if [ "${LLMISVC}" = "true" ]; then
    log_info "LLMISVC is enabled"
    CRD_DIR_NAME="kserve-llmisvc-crd"
    CORE_DIR_NAME="kserve-llmisvc-resources"
    KSERVE_CRD_RELEASE_NAME="kserve-llmisvc-crd"
    KSERVE_RELEASE_NAME="kserve-llmisvc-resources"
    TARGET_DEPLOYMENT_NAMES=("kserve-llmisvc-controller-manager")
fi

if [ "${SET_KSERVE_VERSION}" != "" ]; then
    log_info "Setting KServe version to ${SET_KSERVE_VERSION}"
    KSERVE_VERSION="${SET_KSERVE_VERSION}"
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
        helm uninstall "${KSERVE_CRD_RELEASE_NAME}" -n "${KSERVE_NAMESPACE}" --namespace "${KSERVE_NAMESPACE}" 2>/dev/null || true
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
        
        # Update default version in values.yaml
        log_info "Updating default version in values.yaml to ${KSERVE_VERSION}"
        sed -i -e "s/*defaultVersion*/${KSERVE_VERSION}/g" ${CHARTS_DIR}/${CORE_DIR_NAME}/values.yaml
            
        # Install KServe CRDs from local chart
        log_info "Installing KServe CRDs..."
        helm upgrade --install "${KSERVE_CRD_RELEASE_NAME}" "${CHARTS_DIR}/${CRD_DIR_NAME}" \
            --namespace "${KSERVE_NAMESPACE}" \
            --create-namespace \
            --wait \
            ${KSERVE_CRD_EXTRA_ARGS:-}

        # Install KServe resources from local chart
        log_info "Installing KServe resources..."
        helm upgrade --install "${KSERVE_RELEASE_NAME}" "${CHARTS_DIR}/${CORE_DIR_NAME}" \
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
        helm upgrade --install "${KSERVE_CRD_RELEASE_NAME}" \
            oci://ghcr.io/kserve/charts/${CRD_DIR_NAME} \
            --version "${KSERVE_VERSION}" \
            --namespace "${KSERVE_NAMESPACE}" \
            --create-namespace \
            --wait \
            ${KSERVE_CRD_EXTRA_ARGS:-}

        # Install KServe resources
        log_info "Installing KServe resources..."
        if ! helm upgrade --install "${KSERVE_RELEASE_NAME}" \
            oci://ghcr.io/kserve/charts/${KSERVE_RELEASE_NAME} \
            --version "${KSERVE_VERSION}" \
            --namespace "${KSERVE_NAMESPACE}" \
            --create-namespace \
            --wait \
            ${KSERVE_EXTRA_ARGS:-}; then

            # If installation fails, try using helm upgrade after kserve controller is Ready
            log_info "Install failed, attempting upgrade instead..."
        
            for deploy in "${TARGET_DEPLOYMENT_NAMES[@]}"; do
                    wait_for_deployment "${KSERVE_NAMESPACE}" "${deploy}" "120s"
            done
            if ! helm upgrade "${KSERVE_RELEASE_NAME}" \
                oci://ghcr.io/kserve/charts/${KSERVE_RELEASE_NAME} \
                --version "${KSERVE_VERSION}" \
                --namespace "${KSERVE_NAMESPACE}" \
                --wait \
                ${KSERVE_EXTRA_ARGS:-}; then

                log_error "Failed to install/upgrade KServe ${KSERVE_VERSION}"
                exit 1
            fi
        fi

        log_success "Successfully installed KServe ${KSERVE_VERSION}"
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
}

if [ "$UNINSTALL" = true ]; then
    uninstall
    exit 0
fi

install
