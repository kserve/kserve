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
# Usage: manage.kserve-kustomize.sh [--reinstall|--uninstall|--force-upgrade]
#   or:  REINSTALL=true manage.kserve-kustomize.sh
#   or:  UNINSTALL=true manage.kserve-kustomize.sh
#   or:  FORCE_UPGRADE=true manage.kserve-kustomize.sh
#
# Environment variables:
#   ENABLE_KSERVE - Enable KServe controller (default: true)
#   ENABLE_LLMISVC - Enable LLM Inference Service Controller (default: false)
#   ENABLE_LOCALMODEL - Enable Local Model controller (default: false)
#
#   SET_KSERVE_REGISTRY - Custom image registry (e.g., quay.io/jooholee)
#                         Replaces default 'kserve/' registry in all images
#
#   KSERVE_OVERLAY_DIR - Custom overlay directory (relative to config/overlays/)
#                        If set, uses specified overlay instead of auto-selected ones
#                        Special overlays:
#                          - test: KServe + LocalModel (for testing)
#
#   DEPLOYMENT_MODE - Default deployment mode (default: Knative)
#
#   USE_LOCAL_CONFIGMAP - Use local configmap from config/configmap as-is (default: false)
#                         If true, ignores DEPLOYMENT_MODE and other config updates
#
#   GATEWAY_NETWORK_LAYER - false, envoy, istio (default: false)
#                           if it is not false, ENABLE_GATEWAY_API will be set true
#
#   INSTALL_RUNTIMES - Install ClusterServingRuntimes (default: same as ENABLE_KSERVE)
#   INSTALL_LLMISVC_CONFIGS - Install LLMISVC configs (default: same as ENABLE_LLMISVC)
#
# This script installs KServe directly from the local config directories
# using kubectl kustomize. 
#
# Examples:
#   # Install KServe only (default)
#   ./manage.kserve-kustomize.sh
#
#   # Install LLMISVC only
#   ENABLE_KSERVE=false ENABLE_LLMISVC=true ./manage.kserve-kustomize.sh
#
#   # Install both KServe and LLMISVC
#   ENABLE_KSERVE=true ENABLE_LLMISVC=true ./manage.kserve-kustomize.sh
#
#   # Install all three (KServe, LLMISVC, LocalModel)
#   ENABLE_KSERVE=true ENABLE_LLMISVC=true ENABLE_LOCALMODEL=true ./manage.kserve-kustomize.sh
#
#   # Use Standard deployment mode
#   DEPLOYMENT_MODE=Standard ./manage.kserve-kustomize.sh
#
#   # Use custom test overlay (KServe + LocalModel)
#   KSERVE_OVERLAY_DIR=test DEPLOYMENT_MODE=Standard ./manage.kserve-kustomize.sh
#
#   # Use custom image registry (e.g., for local development)
#   SET_KSERVE_REGISTRY=quay.io/jooholee SET_KSERVE_VERSION=v0.16.0 ./manage.kserve-kustomize.sh
#
#   # Reinstall everything
#   ./manage.kserve-kustomize.sh --reinstall

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

check_cli_exist kubectl

# VARIABLES
# KSERVE_NAMESPACE is defined in global-vars.env
INSTALL_MODE="kustomize"
SET_KSERVE_VERSION="${SET_KSERVE_VERSION:-}"
SET_KSERVE_REGISTRY="${SET_KSERVE_REGISTRY:-}"

# Override KSERVE_VERSION if SET_KSERVE_VERSION is provided
if [ -n "${SET_KSERVE_VERSION}" ]; then
    KSERVE_VERSION="${SET_KSERVE_VERSION}"
fi

ENABLE_KSERVE="${ENABLE_KSERVE:-${KSERVE:-true}}"
ENABLE_LLMISVC="${ENABLE_LLMISVC:-${LLMISVC:-false}}"
ENABLE_LOCALMODEL="${ENABLE_LOCALMODEL:-${LOCALMODEL:-false}}"
KSERVE_OVERLAY_DIR="${KSERVE_OVERLAY_DIR:-}"
USE_LOCAL_CONFIGMAP="${USE_LOCAL_CONFIGMAP:-false}"
# DEPLOYMENT_MODE, GATEWAY_NETWORK_LAYER, LLMISVC, EMBED_MANIFESTS are defined in global-vars.env
KSERVE_INSTALLED="${KSERVE_INSTALLED:-0}"
LLMISVC_INSTALLED="${LLMISVC_INSTALLED:-0}"
INSTALL_RUNTIMES="${INSTALL_RUNTIMES:-${ENABLE_KSERVE:-false}}"
INSTALL_LLMISVC_CONFIGS="${INSTALL_LLMISVC_CONFIGS:-${ENABLE_LLMISVC:-false}}"
FORCE_UPGRADE="${FORCE_UPGRADE:-false}"

TARGET_CRD_DIRS=()
TARGET_DEPLOYMENT_NAMES=()
TARGET_OVERLAY_DIRS=()
TARGET_CRDS_TO_VERIFY=()
# VARIABLES END

# INCLUDE_IN_GENERATED_SCRIPT_START
KSERVE_CRDS="inferenceservices.serving.kserve.io servingruntimes.serving.kserve.io clusterservingruntimes.serving.kserve.io inferencegraphs.serving.kserve.io trainedmodels.serving.kserve.io"
LLMISVC_CRDS="llminferenceservices.serving.kserve.io llminferenceserviceconfigs.serving.kserve.io"
LOCALMODEL_CRDS="localmodelcaches.serving.kserve.io localmodelnodegroups.serving.kserve.io localmodelnodes.serving.kserve.io"
KSERVE_CONFIG_DIR="${REPO_ROOT}/config/overlays/standalone/kserve"
LLMISVC_CONFIG_DIR="${REPO_ROOT}/config/overlays/standalone/llmisvc"
LOCALMODEL_CONFIG_DIR="${REPO_ROOT}/config/overlays/addons/localmodel"
RUNTIMES_DIR="${REPO_ROOT}/config/runtimes"

# Create temporary overlay if version/registry override is needed
if ! is_positive "$EMBED_MANIFESTS" && [ -z "${KSERVE_OVERLAY_DIR}" ] && ([ -n "${SET_KSERVE_VERSION}" ] || [ -n "${SET_KSERVE_REGISTRY}" ]); then
    TEMP_OVERLAY_DIR="${REPO_ROOT}/config/overlays/temp"
    TEMPLATE_DIR="${REPO_ROOT}/config/overlays/version-template"

    log_info "Creating temporary overlay from template: ${TEMP_OVERLAY_DIR}"

    # Copy template
    rm -rf "${TEMP_OVERLAY_DIR}"
    cp -r "${TEMPLATE_DIR}" "${TEMP_OVERLAY_DIR}"

    # Replace version/registry placeholders
    VERSION="${SET_KSERVE_VERSION:-latest}"
    REGISTRY="${SET_KSERVE_REGISTRY:-kserve}"

    find "${TEMP_OVERLAY_DIR}" -type f -name "*.yaml" -exec sed -i \
        -e "s/latest/${VERSION}/g" \
        -e "s|kserve/|${REGISTRY}/|g" {} \;

    # Uncomment components/patches based on ENABLE_* flags
    if is_positive "${ENABLE_KSERVE}"; then
        sed -i 's/#ENABLE_KSERVE //' "${TEMP_OVERLAY_DIR}/kustomization.yaml"
    fi

    if is_positive "${ENABLE_LLMISVC}"; then
        sed -i 's/#ENABLE_LLMISVC //' "${TEMP_OVERLAY_DIR}/kustomization.yaml"
    fi

    if is_positive "${ENABLE_LOCALMODEL}"; then
        sed -i 's/#ENABLE_LOCALMODEL //' "${TEMP_OVERLAY_DIR}/kustomization.yaml"
    fi

    # Use temporary overlay
    KSERVE_OVERLAY_DIR="temp"
    log_success "Temporary overlay created successfully"
fi

if [ -n "${KSERVE_OVERLAY_DIR}" ]; then
    TARGET_OVERLAY_DIRS+=("${REPO_ROOT}/config/overlays/${KSERVE_OVERLAY_DIR}")
    if [ "${KSERVE_OVERLAY_DIR}" == "test" ]; then
        # Auto-enable localmodel for test overlay
        ENABLE_LOCALMODEL="true"

        # Update test overlay image tags if version is set
        if [ -n "${SET_KSERVE_VERSION}" ]; then
            log_info "Updating test overlay image tags to ${SET_KSERVE_VERSION}..."
            sed -i -e "s/latest/${SET_KSERVE_VERSION}/g" config/overlays/test/configmap/inferenceservice.yaml
            sed -i -e "s/latest/${SET_KSERVE_VERSION}/g" config/overlays/test/clusterresources/kustomization.yaml
        fi

        TARGET_CRD_DIRS+=("${REPO_ROOT}/config/crd/full")
        TARGET_CRD_DIRS+=("${REPO_ROOT}/config/crd/full/localmodel")
        TARGET_CRDS_TO_VERIFY+=("${KSERVE_CRDS}")
        TARGET_CRDS_TO_VERIFY+=("${LOCALMODEL_CRDS}")
        TARGET_DEPLOYMENT_NAMES+=("kserve-controller-manager")
        TARGET_DEPLOYMENT_NAMES+=("kserve-localmodel-controller-manager")
    elif [ "${KSERVE_OVERLAY_DIR}" == "test-llmisvc" ]; then
        TARGET_CRD_DIRS+=("${REPO_ROOT}/config/crd/full/llmisvc")
        TARGET_CRDS_TO_VERIFY+=("${LLMISVC_CRDS}")
        TARGET_DEPLOYMENT_NAMES+=("llmisvc-controller-manager")
    elif [ "${KSERVE_OVERLAY_DIR}" == "temp" ]; then
        RUNTIMES_DIR="${REPO_ROOT}/config/overlays/temp/cluster-resources"
        if is_positive "${ENABLE_KSERVE}"; then
            TARGET_CRD_DIRS+=("${REPO_ROOT}/config/crd/full")
            TARGET_CRDS_TO_VERIFY+=("${KSERVE_CRDS}")
            TARGET_DEPLOYMENT_NAMES+=("kserve-controller-manager")
        fi
        if is_positive "${ENABLE_LLMISVC}"; then
            TARGET_CRD_DIRS+=("${REPO_ROOT}/config/crd/full/llmisvc")
            TARGET_CRDS_TO_VERIFY+=("${LLMISVC_CRDS}")
            TARGET_DEPLOYMENT_NAMES+=("llmisvc-controller-manager")
        fi
        if is_positive "${ENABLE_LOCALMODEL}"; then
            TARGET_CRD_DIRS+=("${REPO_ROOT}/config/crd/full/localmodel")
            TARGET_CRDS_TO_VERIFY+=("${LOCALMODEL_CRDS}")
            TARGET_DEPLOYMENT_NAMES+=("kserve-localmodel-controller-manager")
        fi
    fi
else
    if is_positive "${ENABLE_KSERVE}"; then
        TARGET_CRD_DIRS+=("${REPO_ROOT}/config/crd/full")
        TARGET_CRDS_TO_VERIFY+=("${KSERVE_CRDS}")
        TARGET_DEPLOYMENT_NAMES+=("kserve-controller-manager")
        if [ "${LLMISVC_INSTALLED}" = "1" ]; then
            KSERVE_CONFIG_DIR="${REPO_ROOT}/config/overlays/addons/kserve"
        fi
        TARGET_OVERLAY_DIRS+=("${KSERVE_CONFIG_DIR}")
    fi

    if is_positive "${ENABLE_LLMISVC}"; then
        TARGET_CRD_DIRS+=("${REPO_ROOT}/config/crd/full/llmisvc")
        TARGET_CRDS_TO_VERIFY+=("${LLMISVC_CRDS}")
        TARGET_DEPLOYMENT_NAMES+=("llmisvc-controller-manager")
        if [ "${KSERVE_INSTALLED}" = "1" ]; then
            LLMISVC_CONFIG_DIR="${REPO_ROOT}/config/overlays/addons/llmisvc"
        fi
        TARGET_OVERLAY_DIRS+=("${LLMISVC_CONFIG_DIR}")
    fi

    if is_positive "${ENABLE_LOCALMODEL}"; then
        TARGET_CRD_DIRS+=("${REPO_ROOT}/config/crd/full/localmodel")
        TARGET_CRDS_TO_VERIFY+=("${LOCALMODEL_CRDS}")
        TARGET_OVERLAY_DIRS+=("${LOCALMODEL_CONFIG_DIR}")
        TARGET_DEPLOYMENT_NAMES+=("kserve-localmodel-controller-manager")
    fi
fi

# Add ClusterStorageContainer CRD if either KServe or LLMISVC is enabled
if is_positive "${ENABLE_KSERVE}" || is_positive "${ENABLE_LLMISVC}"; then
    TARGET_CRD_DIRS+=("${REPO_ROOT}/config/crd/full/clusterstoragecontainer")
    TARGET_CRDS_TO_VERIFY+=("clusterstoragecontainers.serving.kserve.io")
fi
# INCLUDE_IN_GENERATED_SCRIPT_END

uninstall() {
    log_info "Uninstalling KServe..."

    # EMBED_MANIFESTS: use embedded manifests
    if is_positive "$EMBED_MANIFESTS"; then
        if type uninstall_kserve_manifest &>/dev/null; then
            uninstall_kserve_manifest
        else
            log_error "EMBED_MANIFESTS enabled but uninstall_kserve_manifest function not found"
            log_error "This script should be called from a generated installation script"
            exit 1
        fi
    else
        # Uninstall Runtimes and LLMISVC configs first
        if is_positive "${INSTALL_RUNTIMES}"; then
            log_info "Uninstalling ClusterServingRuntimes..."
            kubectl delete -k config/runtimes --force --grace-period=0 2>/dev/null || true
        fi
        if is_positive "${INSTALL_LLMISVC_CONFIGS}"; then
            log_info "Uninstalling LLMISVC configs..."
            kubectl delete -k config/llmisvcconfig --force --grace-period=0 2>/dev/null || true
        fi

        # Uninstall overlay resources in reverse order
        for ((i=${#TARGET_OVERLAY_DIRS[@]}-1; i>=0; i--)); do
            log_info "Uninstalling resources from ${TARGET_OVERLAY_DIRS[$i]}..."
            kubectl kustomize "${TARGET_OVERLAY_DIRS[$i]}" | kubectl delete -f - --force --grace-period=0 2>/dev/null || true
        done

        # Uninstall CRDs in reverse order
        for ((i=${#TARGET_CRD_DIRS[@]}-1; i>=0; i--)); do
            log_info "Uninstalling CRDs from ${TARGET_CRD_DIRS[$i]}..."
            kubectl kustomize "${TARGET_CRD_DIRS[$i]}" | kubectl delete -f - --force --grace-period=0 2>/dev/null || true
        done
    fi

    kubectl delete all --all -n "${KSERVE_NAMESPACE}" --force --grace-period=0 2>/dev/null || true
    kubectl delete namespace "${KSERVE_NAMESPACE}" --wait=true --timeout=60s --force --grace-period=0 2>/dev/null || true
    log_success "KServe uninstalled"
}

install() {
    # Determine installation status
    determine_shared_resources_config "${INSTALL_MODE}" "${ENABLE_KSERVE}" "${ENABLE_LLMISVC}"

    # Check if already installed
    local already_installed=false
    if [ "${KSERVE_INSTALLED}" = "1" ] || [ "${LLMISVC_INSTALLED}" = "1" ]; then
        already_installed=true
    fi

    if [ "${already_installed}" = "true" ]; then
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

    # EMBED_MANIFESTS: use embedded manifests from generated script
    if is_positive "$EMBED_MANIFESTS"; then
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
        log_info "Installing KServe via Kustomize..."

        # Install CRDs and wait for them
        log_info "Installing KServe CRDs..."
        for i in "${!TARGET_CRD_DIRS[@]}"; do
            crd_dir="${TARGET_CRD_DIRS[$i]}"
            log_info "  - Installing CRDs from ${crd_dir}..."
            kustomize build "${crd_dir}" | kubectl apply --server-side --force-conflicts -f -

            # Collect CRDs to verify
            crds="${TARGET_CRDS_TO_VERIFY[$i]}"
            if [ -n "${crds}" ]; then
                log_info "Waiting for required CRDs..."
                wait_for_crds "60s" ${crds}
            fi
        done

        # Install resources from overlays
        log_info "Installing KServe resources..."
        for i in "${!TARGET_OVERLAY_DIRS[@]}"; do
            overlay_dir="${TARGET_OVERLAY_DIRS[$i]}"

            # Install overlay
            log_info "  - Installing resources from ${overlay_dir}..."
            kustomize build "${overlay_dir}" | kubectl apply --server-side -f -

            # Wait for corresponding deployment
            if [ ${#TARGET_DEPLOYMENT_NAMES[@]} -gt $i ] && [ -n "${TARGET_DEPLOYMENT_NAMES[$i]}" ]; then
                for d in ${TARGET_DEPLOYMENT_NAMES[$i]}; do
                    wait_for_deployment "${KSERVE_NAMESPACE}" "${d}" "300s"
                done
            fi
        done

        # Cleanup temporary overlay
        if [ "${KSERVE_OVERLAY_DIR}" = "temp" ]; then
            rm -rf "${REPO_ROOT}/config/overlays/temp"
            log_info "Temporary overlay directory cleaned up"
        fi
    fi

    if ! is_positive "${USE_LOCAL_CONFIGMAP}"; then
        # Build list of config updates
        local config_updates=()

        # Update deployment mode if needed
        if [ "${DEPLOYMENT_MODE}" = "Standard" ] || [ "${DEPLOYMENT_MODE}" = "RawDeployment" ]; then
            log_info "Adding deployment mode update: ${DEPLOYMENT_MODE}"
            config_updates+=("deploy.defaultDeploymentMode=${DEPLOYMENT_MODE}")
        fi

        # Enable Gateway API for KServe(ISVC) if needed
        if [ "${GATEWAY_NETWORK_LAYER}" != "false" ] && ! is_positive "${ENABLE_LLMISVC}"; then
            log_info "Adding Gateway API updates: enableGatewayApi=true, ingressClassName=${GATEWAY_NETWORK_LAYER}"
            config_updates+=("ingress.enableGatewayApi=true")
            config_updates+=("ingress.ingressClassName=${GATEWAY_NETWORK_LAYER}")
        fi
        if is_positive "${ENABLE_LOCALMODEL}"; then
            log_info "Adding LocalModel updates: enabled=true, defaultJobImage=kserve/storage-initializer:${KSERVE_VERSION}"
            config_updates+=("localModel.enabled=true")
            config_updates+=("localModel.defaultJobImage=kserve/storage-initializer:${KSERVE_VERSION}")
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
            for i in "${!TARGET_OVERLAY_DIRS[@]}"; do
                if [ ${#TARGET_DEPLOYMENT_NAMES[@]} -gt $i ] && [ -n "${TARGET_DEPLOYMENT_NAMES[$i]}" ]; then
                    for d in ${TARGET_DEPLOYMENT_NAMES[$i]}; do
                        wait_for_deployment "${KSERVE_NAMESPACE}" "${d}" "300s"
                    done
                fi
            done
            log_success "KServe configuration updated"
        else
            if is_positive "${ENABLE_LLMISVC}" && ! is_positive "${ENABLE_KSERVE}"; then
                log_info "No configuration updates needed for LLMISVC (GATEWAY_NETWORK_LAYER=${GATEWAY_NETWORK_LAYER})"
            else
                log_info "No configuration updates needed (DEPLOYMENT_MODE=${DEPLOYMENT_MODE}, GATEWAY_NETWORK_LAYER=${GATEWAY_NETWORK_LAYER})"
            fi
        fi
    fi
    log_success "KServe is ready!"
    
    # Wait for kserve webhook endpoint to be ready
    sleep 2
    
    if is_positive "${INSTALL_RUNTIMES}"; then
        log_info "Installing ClusterServingRuntimes..."
        if [ $EMBED_MANIFESTS = "true" ]; then
            create_kserve_runtime_manifests
        else
            retry_command 3 5 kubectl apply --server-side=true -k "${RUNTIMES_DIR}"
        fi
    fi

    if is_positive "${INSTALL_LLMISVC_CONFIGS}"; then
        log_info "Installing LLMISVC configs..."
         if [ $EMBED_MANIFESTS = "true" ]; then
            create_kserve_llmisvcconfig_manifests
        else
            retry_command 3 5 kubectl apply --server-side=true -k "${REPO_ROOT}/config/llmisvcconfig"
        fi
    fi

}

if is_positive "$UNINSTALL"; then
    uninstall
    exit 0
fi

install
