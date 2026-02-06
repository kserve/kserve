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
# Usage: manage.kserve-helm.sh [--reinstall|--uninstall|--update]
#   or:  REINSTALL=true manage.kserve-helm.sh
#   or:  UNINSTALL=true manage.kserve-helm.sh
#   or:  UPDATE=true manage.kserve-helm.sh
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
#   KSERVE_EXTRA_ARGS="--set kserve.controller.containers.manager.resources.limits.cpu=500m" ./manage.kserve-helm.sh
#
#   # Custom controller image for local development
#   USE_LOCAL_CHARTS=true KSERVE_EXTRA_ARGS="--set kserve.controller.containers.manager.tag=local-test --set kserve.controller.containers.manager.imagePullPolicy=Never" ./manage.kserve-helm.sh
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
UPDATE="${UPDATE:-false}"

if [[ "$*" == *"--uninstall"* ]]; then
    UNINSTALL=true    
elif [[ "$*" == *"--reinstall"* ]]; then
    REINSTALL=true
elif [[ "$*" == *"--update"* ]]; then
    UPDATE=true
fi
# INIT END

check_cli_exist helm kubectl

export INSTALL_MODE="helm"

# VARIABLES
# KSERVE_NAMESPACE is defined in global-vars.env
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
RUNTIME_CHARTS_DIR="oci://ghcr.io/kserve/charts"
RUNTIME_CONIFIG_CHART_NAME="kserve-runtime-configs"
# VARIABLES END
   
# INCLUDE_IN_GENERATED_SCRIPT_START
determine_shared_resources_config

if [ "${SET_KSERVE_VERSION}" != "" ]; then
    log_info "Setting KServe version to ${SET_KSERVE_VERSION}"
    KSERVE_VERSION="${SET_KSERVE_VERSION}"
fi

# Build chart arrays based on ENABLE_* flags
if [ "${ENABLE_KSERVE}" = "true" ]; then
    log_info "KServe is enabled"
    CRD_CHARTS+=("kserve-crd")
    RESOURCE_CHARTS+=("kserve-resources")
    RESOURCE_EXTRA_ARGS_LIST+=("${KSERVE_EXTRA_ARGS:-}")
    TARGET_DEPLOYMENT_NAMES+=("kserve-controller-manager")
fi

if [ "${ENABLE_LLMISVC}" = "true" ]; then
    log_info "LLMIsvc is enabled"
    CRD_CHARTS+=("kserve-llmisvc-crd")
    RESOURCE_CHARTS+=("kserve-llmisvc-resources")    
    RESOURCE_EXTRA_ARGS_LIST+=("${LLMISVC_EXTRA_ARGS:-}")
    TARGET_DEPLOYMENT_NAMES+=("llmisvc-controller-manager")
fi

if [ "${ENABLE_LOCALMODEL}" = "true" ]; then
    log_info "LocalModel is enabled"
    CRD_CHARTS+=("kserve-localmodel-crd")
    RESOURCE_CHARTS+=("kserve-localmodel-resources")
    RESOURCE_EXTRA_ARGS_LIST+=("${LOCALMODEL_EXTRA_ARGS:-}")
    TARGET_DEPLOYMENT_NAMES+=("kserve-localmodel-controller-manager")
fi

if [ "${USE_LOCAL_CHARTS}" = "true" ]; then
    RUNTIME_CHARTS_DIR="${CHARTS_DIR}"
fi
# INCLUDE_IN_GENERATED_SCRIPT_END

uninstall() {
    log_info "Uninstalling KServe..."
    if helm list -n "${KSERVE_NAMESPACE}" 2>/dev/null | grep -q "${RUNTIME_CONIFIG_CHART_NAME}"; then
        helm uninstall "${RUNTIME_CONIFIG_CHART_NAME}" -n "${KSERVE_NAMESPACE}"
        log_success "Successfully uninstalled Runtimes/LLMISVC configs"
    fi

    local all_charts=("${RESOURCE_CHARTS[@]}" "${CRD_CHARTS[@]}")
    if [ ${#all_charts[@]} -gt 0 ]; then
        log_info "Uninstalling charts: ${all_charts[*]}"
    else
        log_info "No charts to uninstall"
        return 0
    fi

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
    fi

    log_success "KServe charts uninstalled"
}

install() {
    build_helm_config_args() {
        local config_args=""

        # Update deployment mode if needed
        if [ "${DEPLOYMENT_MODE}" = "Standard" ] || [ "${DEPLOYMENT_MODE}" = "RawDeployment" ]; then
            log_info "Adding deployment mode configuration: ${DEPLOYMENT_MODE}"
            config_args+=" --set inferenceServiceConfig.deploy.defaultDeploymentMode=${DEPLOYMENT_MODE}"
        fi

        # Enable Gateway API for KServe(ISVC) if needed
        if [ "${GATEWAY_NETWORK_LAYER}" != "false" ] && [ "${ENABLE_LLMISVC}" != "true" ]; then
            log_info "Adding Gateway API configuration: enableGatewayApi=true, ingressClassName=${GATEWAY_NETWORK_LAYER}"
            config_args+=" --set inferenceServiceConfig.ingress.enableGatewayApi=true"
            config_args+=" --set inferenceServiceConfig.ingress.ingressClassName=${GATEWAY_NETWORK_LAYER}"
        fi

        if [ "${ENABLE_LOCALMODEL}" = "true" ]; then
            config_args+=" --set inferenceServiceConfig.localModel.enabled=true"
            config_args+=" --set inferenceServiceConfig.localModel.defaultJobImage=kserve/storage-initializer"
            config_args+=" --set inferenceServiceConfig.localModel.defaultJobImageTag=${KSERVE_VERSION}"
        fi
        # Add custom configurations if provided
        if [ -n "${KSERVE_CUSTOM_ISVC_CONFIGS}" ]; then
            log_info "Adding custom configurations: ${KSERVE_CUSTOM_ISVC_CONFIGS}"
            IFS='|' read -ra custom_configs <<< "${KSERVE_CUSTOM_ISVC_CONFIGS}"
            for config in "${custom_configs[@]}"; do
                config_args+=" --set ${config}"
            done
        fi

        echo "${config_args}"
    }

    if [ ${#RESOURCE_CHARTS[@]} -eq 0 ] && [ ${#CRD_CHARTS[@]} -eq 0 ]; then
        log_error "No charts selected for installation. Please enable at least one component (ENABLE_KSERVE, ENABLE_LLMISVC, or ENABLE_LOCALMODEL)."
        exit 1
    fi

    if [ ${#RESOURCE_CHARTS[@]} -gt 0 ]; then
        local main_chart="${RESOURCE_CHARTS[0]}"
        if helm list -n "${KSERVE_NAMESPACE}" 2>/dev/null | grep -q "${main_chart}"; then
            if [ "$REINSTALL" = false ]; then
                if [ "$UPDATE" = true ]; then
                    log_info "Updating KServe..."                    
                else
                    log_info "KServe is already installed. Use --reinstall to reinstall."
                    return 0
                fi
            else
                log_info "Reinstalling KServe..."
                uninstall
            fi
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

        # Install CRD charts
        for chart in "${CRD_CHARTS[@]}"; do
            log_info "Installing ${chart}..."
            helm upgrade -i "${chart}" "${CHARTS_DIR}/${chart}" \
                --namespace "${KSERVE_NAMESPACE}" \
                --create-namespace \
                --wait \
                ${SHARED_EXTRA_ARGS}
        done

        # Build configuration arguments for KServe/LLMIsvc
        local helm_config_args=$(build_helm_config_args)

        # Install resource charts
        for i in "${!RESOURCE_CHARTS[@]}"; do
            local chart="${RESOURCE_CHARTS[$i]}"
            local extra_args="${RESOURCE_EXTRA_ARGS_LIST[$i]}"

            # Apply config args only to first resource chart (kserve or llmisvc)
            local chart_config_args=""
            if [ $i -eq 0 ]; then
                chart_config_args="${helm_config_args}"
            fi

            log_info "Installing ${chart} with version ${KSERVE_VERSION}..."
            helm upgrade -i "${chart}" "${CHARTS_DIR}/${chart}" \
                --namespace "${KSERVE_NAMESPACE}" \
                --create-namespace \
                --wait \
                --set kserve.version="${KSERVE_VERSION}" \
                ${SHARED_EXTRA_ARGS} \
                ${extra_args} \
                ${chart_config_args}
        done
        log_success "Successfully installed KServe using local charts"
    else
        # Install KServe from OCI registry
        log_info "Installing KServe ${KSERVE_VERSION} from OCI registry..."

        # Install CRD charts from OCI registry
        for chart in "${CRD_CHARTS[@]}"; do
            log_info "Installing ${chart}..."
            helm upgrade -i "${chart}" \
                oci://ghcr.io/kserve/charts/${chart} \
                --version "${KSERVE_VERSION}" \
                --namespace "${KSERVE_NAMESPACE}" \
                --create-namespace \
                --wait \
                ${SHARED_EXTRA_ARGS}
        done

        # Build configuration arguments for KServe/LLMIsvc
        local helm_config_args=$(build_helm_config_args)

        # Install resource charts from OCI registry
        for i in "${!RESOURCE_CHARTS[@]}"; do
            local chart="${RESOURCE_CHARTS[$i]}"
            local extra_args="${RESOURCE_EXTRA_ARGS_LIST[$i]}"

            # Apply config args only to first resource chart (kserve or llmisvc)
            local chart_config_args=""
            if [ $i -eq 0 ]; then
                chart_config_args="${helm_config_args}"
            fi

            log_info "Installing ${chart}..."
            if ! helm upgrade -i "${chart}" \
                oci://ghcr.io/kserve/charts/${chart} \
                --version "${KSERVE_VERSION}" \
                --namespace "${KSERVE_NAMESPACE}" \
                --create-namespace \
                --wait \
                ${SHARED_EXTRA_ARGS} \
                ${extra_args} \
                ${chart_config_args}; then

                # If installation fails, try using helm upgrade after controller is Ready
                log_info "Install failed for ${chart}, attempting upgrade instead..."

                # Wait for the corresponding deployment (if this is the first chart)
                if [ $i -eq 0 ]; then
                    wait_for_deployment "${KSERVE_NAMESPACE}" "${TARGET_DEPLOYMENT_NAMES[0]}" "120s"
                fi

                if ! helm upgrade "${chart}" \
                    oci://ghcr.io/kserve/charts/${chart} \
                    --version "${KSERVE_VERSION}" \
                    --namespace "${KSERVE_NAMESPACE}" \
                    --wait \
                    ${SHARED_EXTRA_ARGS} \
                    ${extra_args} \
                    ${chart_config_args}; then

                    log_error "Failed to install/upgrade ${chart} ${KSERVE_VERSION}"
                    exit 1
                fi
            fi
        done
        log_success "Successfully installed KServe ${KSERVE_VERSION}"
    fi

    log_success "Successfully installed KServe"

    # Wait for all controller managers to be ready
    log_info "Waiting for KServe controllers to be ready..."
    for deploy in "${TARGET_DEPLOYMENT_NAMES[@]}"; do
        wait_for_deployment "${KSERVE_NAMESPACE}" "${deploy}" "300s"
    done

    log_success "KServe is ready!"
    if [ ${INSTALL_RUNTIMES} = "true" ] || [ ${INSTALL_LLMISVC_CONFIGS} = "true" ]; then
        log_info "Installing Runtimes(${INSTALL_RUNTIMES}) and LLMISVC configs(${INSTALL_LLMISVC_CONFIGS})..."
        helm upgrade -i ${RUNTIME_CONIFIG_CHART_NAME} \
            ${RUNTIME_CHARTS_DIR}/${RUNTIME_CONIFIG_CHART_NAME} \
            --namespace "${KSERVE_NAMESPACE}" \
            --create-namespace \
            --wait \
            --set kserve.version="${KSERVE_VERSION}" \
            --set runtimes.enabled=${INSTALL_RUNTIMES} \
            --set llmisvcConfigs.enabled=${INSTALL_LLMISVC_CONFIGS}
        log_success "Successfully installed Runtimes/LLMISVC configs"
    fi
}

if [ "$UNINSTALL" = true ]; then
    uninstall
    exit 0
fi

install
