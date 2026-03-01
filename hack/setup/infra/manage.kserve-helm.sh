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
# Usage: manage.kserve-helm.sh [--reinstall|--uninstall|--force-upgrade]
#   or:  REINSTALL=true manage.kserve-helm.sh
#   or:  UNINSTALL=true manage.kserve-helm.sh
#   or:  FORCE_UPGRADE=true manage.kserve-helm.sh
#
# Environment variables:
#   USE_LOCAL_CHARTS      - Use local charts instead of OCI registry (default: false)
#   SET_KSERVE_VERSION    - KServe version to install (default: from kserve-deps.env)
#
#   ENABLE_KSERVE         - Enable KServe controller (default: true)
#   ENABLE_LLMISVC        - Enable LLM Inference Service Controller (default: false)
#   ENABLE_LOCALMODEL     - Enable LocalModel Controller (default: false)
#
#   INSTALL_RUNTIMES      - Install ClusterServingRuntimes (default: based on ENABLE_KSERVE)
#   INSTALL_LLMISVC_CONFIGS - Install LLMInferenceServiceConfigs (default: based on ENABLE_LLMISVC)
#
#   SHARED_EXTRA_ARGS     - Additional helm upgrade -i arguments applied to ALL charts
#   KSERVE_EXTRA_ARGS     - Additional helm upgrade -i arguments for KServe resources
#   LLMISVC_EXTRA_ARGS    - Additional helm upgrade -i arguments for LLMIsvc resources
#   LOCALMODEL_EXTRA_ARGS - Additional helm upgrade -i arguments for LocalModel resources
#
#   Legacy (for backward compatibility):
#   LLMISVC               - Same as ENABLE_LLMISVC (deprecated, use ENABLE_LLMISVC)
#   LOCALMODEL            - Same as ENABLE_LOCALMODEL (deprecated, use ENABLE_LOCALMODEL)
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
#                           if it is not false, enableGatewayApi will be set true
#
# Examples:
#   # Install KServe only (default)
#   ./manage.kserve-helm.sh
#
#   # Install LLMISVC only
#   ENABLE_KSERVE=false ENABLE_LLMISVC=true ./manage.kserve-helm.sh
#
#   # Install both KServe and LLMISVC
#   ENABLE_KSERVE=true ENABLE_LLMISVC=true ./manage.kserve-helm.sh
#
#   # Install all three (KServe, LLMISVC, LocalModel)
#   ENABLE_KSERVE=true ENABLE_LLMISVC=true ENABLE_LOCALMODEL=true ./manage.kserve-helm.sh
#
#   # Install specific version from OCI registry
#   SET_KSERVE_VERSION=v0.15.0 ./manage.kserve-helm.sh
#
#   # Install from local charts for development
#   USE_LOCAL_CHARTS=true ./manage.kserve-helm.sh
#
#   # Use Standard deployment mode
#   DEPLOYMENT_MODE=Standard ./manage.kserve-helm.sh
#
#   # Apply shared arguments to all charts
#   SHARED_EXTRA_ARGS="--timeout 10m" ./manage.kserve-helm.sh
#
#   # Custom resource limits for KServe only
#   KSERVE_EXTRA_ARGS="--set kserve.controller.resources.limits.cpu=500m" ./manage.kserve-helm.sh
#
#   # Custom controller image for local development
#   USE_LOCAL_CHARTS=true KSERVE_EXTRA_ARGS="--set kserve.controller.tag=local-test --set kserve.controller.imagePullPolicy=Never" ./manage.kserve-helm.sh
#
#   # Install without ClusterServingRuntimes
#   INSTALL_RUNTIMES=false ./manage.kserve-helm.sh
#
#   # Reinstall everything (based on ENABLE_* flags)
#   ./manage.kserve-helm.sh --reinstall

# INIT
SCRIPT_DIR="$(cd "$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")" && pwd)"

source "${SCRIPT_DIR}/../common.sh"

REINSTALL="${REINSTALL:-false}"
UNINSTALL="${UNINSTALL:-false}"
FORCE_UPGRADE="${FORCE_UPGRADE:-false}"

if [[ "$*" == *"--uninstall"* ]]; then
    UNINSTALL=true
elif [[ "$*" == *"--reinstall"* ]]; then
    REINSTALL=true
elif [[ "$*" == *"--force-upgrade"* ]]; then
    FORCE_UPGRADE=true
fi
# INIT END

check_cli_exist helm kubectl


# VARIABLES
# KSERVE_NAMESPACE is defined in global-vars.env
INSTALL_MODE="helm"
USE_LOCAL_CHARTS="${USE_LOCAL_CHARTS:-false}"
CHARTS_DIR="${REPO_ROOT}/charts"
SET_KSERVE_VERSION="${SET_KSERVE_VERSION:-}"
SHARED_EXTRA_ARGS="${SHARED_EXTRA_ARGS:-}"

ENABLE_KSERVE="${ENABLE_KSERVE:-true}"
ENABLE_LLMISVC="${ENABLE_LLMISVC:-${LLMISVC:-false}}"
ENABLE_LOCALMODEL="${ENABLE_LOCALMODEL:-${LOCALMODEL:-false}}"

# Arrays for managing multiple charts
CRD_CHARTS=()
RESOURCE_CHARTS=()
RESOURCE_EXTRA_ARGS_LIST=()
TARGET_DEPLOYMENT_NAMES=()

# DEPLOYMENT_MODE, GATEWAY_NETWORK_LAYER, EMBED_MANIFESTS are defined in global-vars.env
INSTALL_RUNTIMES="${INSTALL_RUNTIMES:-${ENABLE_KSERVE:-false}}"
INSTALL_LLMISVC_CONFIGS="${INSTALL_LLMISVC_CONFIGS:-${ENABLE_LLMISVC:-false}}"
RUNTIME_CONFIG_CHART_NAME="kserve-runtime-configs"
# VARIABLES END

# INCLUDE_IN_GENERATED_SCRIPT_START
determine_shared_resources_config "${INSTALL_MODE}" "${ENABLE_KSERVE}" "${ENABLE_LLMISVC}"

if [ "${SET_KSERVE_VERSION}" != "" ]; then
    log_info "Setting KServe version to ${SET_KSERVE_VERSION}"
    KSERVE_VERSION="${SET_KSERVE_VERSION}"
fi

# Build chart arrays based on ENABLE_* flags
if is_positive "${ENABLE_KSERVE}"; then
    log_info "KServe is enabled"
    CRD_CHARTS+=("kserve-crd")
    RESOURCE_CHARTS+=("kserve-resources")
    RESOURCE_EXTRA_ARGS_LIST+=("${KSERVE_EXTRA_ARGS:-}")
    TARGET_DEPLOYMENT_NAMES+=("kserve-controller-manager")
fi

if is_positive "${ENABLE_LLMISVC}"; then
    log_info "LLMIsvc is enabled"
    CRD_CHARTS+=("kserve-llmisvc-crd")
    RESOURCE_CHARTS+=("kserve-llmisvc-resources")
    RESOURCE_EXTRA_ARGS_LIST+=("${LLMISVC_EXTRA_ARGS:-}")
    TARGET_DEPLOYMENT_NAMES+=("llmisvc-controller-manager")
fi

if is_positive "${ENABLE_LOCALMODEL}"; then
    log_info "LocalModel is enabled"
    CRD_CHARTS+=("kserve-localmodel-crd")
    RESOURCE_CHARTS+=("kserve-localmodel-resources")
    RESOURCE_EXTRA_ARGS_LIST+=("${LOCALMODEL_EXTRA_ARGS:-}")
    TARGET_DEPLOYMENT_NAMES+=("kserve-localmodel-controller-manager")
fi
# INCLUDE_IN_GENERATED_SCRIPT_END

uninstall() {
    log_info "Uninstalling KServe..."
    if helm list -n "${KSERVE_NAMESPACE}" 2>/dev/null | grep -q "${RUNTIME_CONFIG_CHART_NAME}"; then
        helm uninstall "${RUNTIME_CONFIG_CHART_NAME}" -n "${KSERVE_NAMESPACE}"
        log_success "Successfully uninstalled Runtimes/LLMISVC configs"
    fi

    local all_charts=("${RESOURCE_CHARTS[@]}" "${CRD_CHARTS[@]}")
    if [ ${#all_charts[@]} -gt 0 ]; then
        log_info "Uninstalling charts: ${all_charts[*]}"
    else
        log_info "No charts to uninstall"
        return 0
    fi

    for ((i=${#RESOURCE_CHARTS[@]}-1; i>=0; i--)); do
        local chart="${RESOURCE_CHARTS[$i]}"
        log_info "Uninstalling ${chart}..."
        helm uninstall "${chart}" -n "${KSERVE_NAMESPACE}" 2>/dev/null || true
    done

    # Then uninstall CRD charts (reverse order)
    for ((i=${#CRD_CHARTS[@]}-1; i>=0; i--)); do
        local chart="${CRD_CHARTS[$i]}"
        log_info "Uninstalling ${chart}..."
        helm uninstall "${chart}" -n "${KSERVE_NAMESPACE}" 2>/dev/null || true
    done

    log_success "KServe charts uninstalled"
}

install() {
    build_helm_config_args() {
        local -a config_args=()

        # Update deployment mode if needed
        if [ "${DEPLOYMENT_MODE}" = "Standard" ] || [ "${DEPLOYMENT_MODE}" = "RawDeployment" ]; then
            log_info "Adding deployment mode configuration: ${DEPLOYMENT_MODE}"
            config_args+=(--set "kserve.controller.deploymentMode=${DEPLOYMENT_MODE}")
        fi

        # Enable Gateway API for KServe(ISVC) if needed
        if [ "${GATEWAY_NETWORK_LAYER}" != "false" ] && ! is_positive "${ENABLE_LLMISVC}"; then
            log_info "Adding Gateway API configuration: enableGatewayApi=true, ingressClassName=${GATEWAY_NETWORK_LAYER}"
            config_args+=(--set "kserve.controller.gateway.ingressGateway.enableGatewayApi=true")
            config_args+=(--set "kserve.controller.gateway.ingressGateway.className=${GATEWAY_NETWORK_LAYER}")
        fi

        if is_positive "${ENABLE_LOCALMODEL}"; then
            config_args+=(--set "kserve.localModel.enabled=true")
            config_args+=(--set "kserve.localModel.defaultJobImage=kserve/storage-initializer")
            config_args+=(--set "kserve.localModel.defaultJobImageTag=${KSERVE_VERSION}")
        fi
        # Add custom configurations if provided
        if [ -n "${KSERVE_CUSTOM_ISVC_CONFIGS}" ]; then
            log_info "Adding custom configurations: ${KSERVE_CUSTOM_ISVC_CONFIGS}"
            IFS='|' read -ra custom_configs <<< "${KSERVE_CUSTOM_ISVC_CONFIGS}"
            for config in "${custom_configs[@]}"; do
                config_args+=(--set "${config}")
            done
        fi

        # Only print if array has elements
        if [ ${#config_args[@]} -gt 0 ]; then
            printf '%s\n' "${config_args[@]}"
        fi
    }

    if [ ${#RESOURCE_CHARTS[@]} -eq 0 ] && [ ${#CRD_CHARTS[@]} -eq 0 ]; then
        log_error "No charts selected for installation. Please enable at least one component (ENABLE_KSERVE, ENABLE_LLMISVC, or ENABLE_LOCALMODEL)."
        exit 1
    fi

    if [ ${#RESOURCE_CHARTS[@]} -gt 0 ]; then
        local main_chart="${RESOURCE_CHARTS[0]}"
        # Use exact match for helm release name to avoid partial matches
        if helm list -n "${KSERVE_NAMESPACE}" -q 2>/dev/null | grep -x "${main_chart}" &>/dev/null; then
            if ! is_positive "$REINSTALL"; then
                if is_positive "$FORCE_UPGRADE"; then
                    log_info "Force upgrading KServe..."
                else
                    log_info "KServe is already installed. Use --reinstall to reinstall or --force-upgrade to upgrade."
                    return 0
                fi
            else
                log_info "Reinstalling KServe..."
                uninstall
            fi
        fi
    fi

    # Determine chart repository
    local CHART_REPO="oci://ghcr.io/kserve/charts"
    if is_positive "${USE_LOCAL_CHARTS}"; then
        CHART_REPO="${CHARTS_DIR}"
        log_info "Installing KServe using local charts..."
        log_info "📍 Using local charts from ${CHARTS_DIR}/"
    else
        log_info "Installing KServe ${KSERVE_VERSION} from OCI registry..."
    fi

    # Build chart version flag (only for remote charts, skip for 'latest')
    local VERSION_FLAG=""
    if ! is_positive "${USE_LOCAL_CHARTS}"; then
        VERSION_FLAG="--version ${KSERVE_VERSION}"       
    fi

    # Install CRD charts
    for chart in "${CRD_CHARTS[@]}"; do
        log_info "Installing ${chart}..."
        helm upgrade -i "${chart}" "${CHART_REPO}/${chart}" \
            --namespace "${KSERVE_NAMESPACE}" \
            --create-namespace \
            --wait \
            ${VERSION_FLAG} \
            ${SHARED_EXTRA_ARGS}
    done

    # Build configuration arguments for KServe/LLMIsvc
    readarray -t helm_config_args < <(build_helm_config_args)

    # Install resource charts
    for i in "${!RESOURCE_CHARTS[@]}"; do
        local chart="${RESOURCE_CHARTS[$i]}"
        local extra_args="${RESOURCE_EXTRA_ARGS_LIST[$i]}"

        # Apply config args only to kserve-resources chart (InferenceService configs)
        local -a extra_helm_args=()
        if [[ "${chart}" == "kserve-resources" ]]; then
            extra_helm_args=("${helm_config_args[@]}")
        fi

        log_info "Installing ${chart}..."
        for attempt in 1 2; do
            if helm upgrade -i "${chart}" "${CHART_REPO}/${chart}" \
                --namespace "${KSERVE_NAMESPACE}" \
                --create-namespace \
                --wait \
                ${VERSION_FLAG} \
                --set kserve.version="${KSERVE_VERSION}" \
                ${SHARED_EXTRA_ARGS} \
                ${extra_args} \
                "${extra_helm_args[@]}"; then
                break
            elif [ $attempt -eq 2 ]; then
                log_error "Failed to install/upgrade ${chart} ${KSERVE_VERSION} after 2 attempts"
                exit 1
            fi
            sleep 5
        done
    done

    log_success "Successfully installed KServe"

    # Wait for all controller managers to be ready
    log_info "Waiting for KServe controllers to be ready..."
    for deploy in "${TARGET_DEPLOYMENT_NAMES[@]}"; do
        wait_for_deployment "${KSERVE_NAMESPACE}" "${deploy}" "300s"
    done

    log_success "KServe is ready!"

    # Install Runtimes and LLMISVC configs if needed
    if is_positive "${INSTALL_RUNTIMES}" || is_positive "${INSTALL_LLMISVC_CONFIGS}"; then
        log_info "Installing Runtimes(${INSTALL_RUNTIMES}) and LLMISVC configs(${INSTALL_LLMISVC_CONFIGS})..."
        helm upgrade -i ${RUNTIME_CONFIG_CHART_NAME} \
            ${CHART_REPO}/${RUNTIME_CONFIG_CHART_NAME} \
            --namespace "${KSERVE_NAMESPACE}" \
            --create-namespace \
            --wait \
            --set kserve.version="${KSERVE_VERSION}" \
            --set kserve.servingruntime.enabled=${INSTALL_RUNTIMES} \
            --set kserve.llmisvcConfigs.enabled=${INSTALL_LLMISVC_CONFIGS}
        log_success "Successfully installed Runtimes/LLMISVC configs"
    fi
}

if is_positive "$UNINSTALL"; then
    uninstall
    exit 0
fi

install
