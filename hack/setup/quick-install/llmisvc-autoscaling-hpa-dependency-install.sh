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

# Install KServe LLM InferenceService HPA autoscaling dependencies (Prometheus, Prometheus Adapter, WVA)
#
# AUTO-GENERATED from: llmisvc-autoscaling-hpa-dependency-install.definition
# DO NOT EDIT MANUALLY
#
# To regenerate:
#   ./scripts/generate-install-script.py llmisvc-autoscaling-hpa-dependency-install.definition
#
# Usage: llmisvc-autoscaling-hpa-dependency-install.sh [--reinstall|--uninstall]

set -o errexit
set -o nounset
set -o pipefail

#================================================
# Helper Functions (from common.sh)
#================================================

# Utility Functions
# ============================================================================

find_repo_root() {
    local current_dir="${1:-$(pwd)}"
    local skip="${2:-false}"

    while [[ "$current_dir" != "/" ]]; do
        if [[ -e "${current_dir}/.git" ]]; then
            echo "$current_dir"
            return 0
        fi
        current_dir="$(dirname "$current_dir")"
    done

    # Git repository not found
    if [[ "$skip" == "true" ]]; then
        log_warning "Could not find git repository root, using current directory: $PWD"
        echo "$PWD"
        return 0
    else
        log_error "Could not find git repository root"
        exit 1
    fi
}

ensure_dir() {
    local dir_path="${1}"

    if [[ -d "${dir_path}" ]]; then
        return 0
    fi

    mkdir -p "${dir_path}"
}

detect_os() {
    local os=""
    case "$(uname -s)" in
        Linux*)  os="linux" ;;
        Darwin*) os="darwin" ;;
        *)       log_error "Unsupported OS detected: $(uname -s)" ; exit 1 ;;
    esac
    echo "$os"
}

detect_arch() {
    local arch=""
    case "$(uname -m)" in
        x86_64)  arch="amd64" ;;
        aarch64|arm64) arch="arm64" ;;
        *)       log_error "Unsupported architecture detected: $(uname -m)" ; exit 1 ;;
    esac
    echo "$arch"
}

cleanup_bin_dir() {
    # Remove BIN_DIR if it was created by this script
    if [[ "${BIN_DIR_CREATED_BY_SCRIPT:-false}" == "true" ]] && [[ -d "${BIN_DIR:-}" ]]; then
        log_info "Cleaning up BIN_DIR: ${BIN_DIR}"
        rm -rf "${BIN_DIR}"
    fi
}

cleanup() {
    # Call all cleanup functions
    cleanup_bin_dir
}

# Set up trap to run cleanup on exit
trap cleanup EXIT

# Color codes (disable if NO_COLOR is set or not a terminal)
if [[ -z "${NO_COLOR:-}" ]] && [[ -t 1 ]]; then
    BLUE='\033[94m'
    GREEN='\033[92m'
    RED='\033[91m'
    YELLOW='\033[93m'
    RESET='\033[0m'
else
    BLUE=''
    GREEN=''
    RED=''
    YELLOW=''
    RESET=''
fi

log_info() {
    echo -e "${BLUE}[INFO]${RESET} $*" >&2
}

log_error() {
    echo -e "${RED}[ERROR]${RESET} $*" >&2
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${RESET} $*" >&2
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${RESET} $*" >&2
}

is_positive() {
  local input_val="${1:-no}"

  case "${input_val}" in
    0|true|True|yes|Yes|y|Y)
      return 0  # Success - truthy
      ;;
    1|false|False|no|No|n|N)
      return 1  # Failure - falsy
      ;;
    *)
      return 2  # Invalid input
      ;;
  esac
}


# ============================================================================
# Infrastructure Installation Helper Functions
# ============================================================================

# Detect the platform (kind/minikube/openshift/kubernetes)
# Returns: kind, minikube, openshift, or kubernetes
detect_platform() {
    # Check for OpenShift
    if kubectl get clusterversion &>/dev/null; then
        echo "openshift"
        return 0
    fi

    # Check for Kind
    local node_hostname
    node_hostname=$(kubectl get nodes -o jsonpath='{.items[0].metadata.labels.kubernetes\.io/hostname}' 2>/dev/null || echo "")
    if [[ "$node_hostname" == *"kind"* ]]; then
        echo "kind"
        return 0
    fi

    # Check for Minikube
    local current_context
    current_context=$(kubectl config current-context 2>/dev/null || echo "")
    if [[ "$current_context" == *"minikube"* ]]; then
        echo "minikube"
        return 0
    fi

    # Default to standard Kubernetes
    echo "kubernetes"
    return 0
}

# Wait for pods to be created (exist)
# Usage: wait_for_pods_created <namespace> <label-selector> [timeout_seconds]
wait_for_pods_created() {
    local namespace="$1"
    local label_selector="$2"
    local timeout="${3:-60}"
    local elapsed=0

    log_info "Waiting for pods with label '$label_selector' in namespace '$namespace' to be created..."

    while true; do
        # Exclude terminating pods by filtering out Terminating status
        local pod_count=$(kubectl get pods -n "$namespace" -l "$label_selector" \
            --no-headers 2>/dev/null | grep -v "Terminating" | wc -l)

        if [ "$pod_count" -gt 0 ]; then
            log_info "Found $pod_count pod(s) with label '$label_selector'"
            return 0
        fi

        if [ $elapsed -ge $timeout ]; then
            log_error "Timeout waiting for pods with label '$label_selector' to be created"
            return 1
        fi

        sleep 2
        elapsed=$((elapsed + 2))
    done
}

# Wait for pods to be ready
# Usage: wait_for_pods_ready <namespace> <label-selector> [timeout]
wait_for_pods_ready() {
    local namespace="$1"
    local label_selector="$2"
    local timeout="${3:-180s}"

    log_info "Waiting for pods with label '$label_selector' in namespace '$namespace' to be ready..."

    # Get list of non-terminating pods and wait for them
    local pods=$(kubectl get pods -n "$namespace" -l "$label_selector" \
        --no-headers 2>/dev/null | grep -v "Terminating" | awk '{print $1}')

    if [ -z "$pods" ]; then
        log_error "No pods found with label '$label_selector' in namespace '$namespace'"
        return 1
    fi

    for pod in $pods; do
        kubectl wait --for=condition=Ready pod/"$pod" -n "$namespace" --timeout="$timeout" || return 1
    done
}

# Wait for pods to be ready (combines both creation and ready checks)
# Usage: wait_for_pods <namespace> <label-selector> [timeout]
wait_for_pods() {
    local namespace="$1"
    local label_selector="$2"
    local timeout="${3:-180s}"

    # Convert timeout to seconds for pod creation check
    local timeout_seconds="${timeout%s}"
    local timeout_created=60

    # If timeout is longer than 60s, use 60s for creation, rest for ready
    # If timeout is shorter, split it
    if [ "$timeout_seconds" -gt 60 ]; then
        timeout_created=60
    else
        timeout_created=$((timeout_seconds / 3))
    fi

    # First, wait for pods to be created
    wait_for_pods_created "$namespace" "$label_selector" "$timeout_created" || return 1

    # Then, wait for pods to be ready
    wait_for_pods_ready "$namespace" "$label_selector" "$timeout" || return 1

    log_success "Pods with label '$label_selector' in namespace '$namespace' are ready!"
}

# Wait for deployment to be available using kubectl wait
# Usage: wait_for_deployment <namespace> <deployment-name> [timeout]
# Note: This uses kubectl wait --for=condition=Available, which checks deployment status directly
wait_for_deployment() {
    local namespace="$1"
    local deployment_name="$2"
    local timeout="${3:-180s}"

    log_info "Waiting for deployment '$deployment_name' in namespace '$namespace' to be available..."
    kubectl wait --timeout="$timeout" -n "$namespace" deployment/"$deployment_name" --for=condition=Available

    if [ $? -eq 0 ]; then
        log_success "Deployment '$deployment_name' in namespace '$namespace' is available!"
    else
        log_error "Deployment '$deployment_name' in namespace '$namespace' failed to become available within $timeout"
        return 1
    fi
}

# Wait for CRD to be established
# Usage: wait_for_crd <crd-name> [timeout]
wait_for_crd() {
    local crd_name="$1"
    local timeout="${2:-60s}"

    log_info "Waiting for CRD '$crd_name' to be established..."

    # Add small delay to allow CRD status to be initialized
    sleep 2

    # Retry logic to handle race condition where .status.conditions may not be initialized yet
    local max_retries=10
    local retry=0
    while [ $retry -lt $max_retries ]; do
        if kubectl wait --for=condition=Established --timeout="$timeout" crd/"$crd_name" 2>/dev/null; then
            return 0
        fi
        retry=$((retry + 1))
        if [ $retry -lt $max_retries ]; then
            log_info "Retry $retry/$max_retries: Waiting for CRD status to be initialized..."
            sleep 3
        fi
    done

    # Final attempt with error output
    kubectl wait --for=condition=Established --timeout="$timeout" crd/"$crd_name"
}

# Wait for multiple CRDs to be established
# Usage: wait_for_crds <timeout> <crd1> <crd2> ...
wait_for_crds() {
    local timeout="$1"
    shift

    for crd in "$@"; do
        wait_for_crd "$crd" "$timeout" || return 1
    done

    log_success "All CRDs are established!"
}

# Update multiple fields in KServe inferenceservice-config ConfigMap
# Usage: update_isvc_config "ingress.enableGatewayApi=true" "deploy.defaultDeploymentMode=Standard"
# Example:
#   update_isvc_config "ingress.enableGatewayApi=true"
#   update_isvc_config "ingress.enableGatewayApi=true" "ingress.className=\"envoy\""
update_isvc_config() {
    if [ $# -eq 0 ]; then
        log_error "No update parameters provided"
        return 1
    fi

    log_info "Updating inferenceservice-config..."

    # Prepare updates as JSON array
    local updates_file
    updates_file=$(mktemp)

    for arg in "$@"; do
        local key="${arg%%=*}"
        local raw_value="${arg#*=}"
        local data_key="${key%%.*}"
        local json_path="${key#*.}"
        local value_json="$raw_value"

        # Smart type detection: auto-quote strings, keep numbers/booleans/JSON as-is
        if [[ ! $raw_value =~ ^\" ]] \
           && [[ ! $raw_value =~ ^-?[0-9]+(\.[0-9]+)?$ ]] \
           && [[ ! $raw_value =~ ^(true|false|null)$ ]] \
           && [[ ! $raw_value =~ ^[{\[] ]]; then
            value_json=$(jq -Rn --arg v "$raw_value" '$v')
        fi

        jq -n \
            --arg data_key "$data_key" \
            --arg path "$json_path" \
            --argjson value "$value_json" \
            '{data_key:$data_key, path:$path, value:$value}' >> "$updates_file"

        log_info "  ✓ $data_key.$json_path = $value_json"
    done

    local updates_json
    updates_json=$(jq -s '.' "$updates_file")
    rm -f "$updates_file"

    # Apply all updates with a single jq invocation
    kubectl get configmap inferenceservice-config -n "${KSERVE_NAMESPACE}" -o json |
        jq --argjson updates "$updates_json" '
            # Helper function to safely set nested paths, creating intermediate objects as needed
            def setpath_safe($parts; $value):
                reduce range(0; ($parts|length)-1) as $i (.;
                    $parts[:$i+1] as $prefix
                    | if getpath($prefix) == null then setpath($prefix; {}) else . end
                )
                | setpath($parts; $value);

            # Apply each update
            reduce $updates[] as $item (.;
                if .data[$item.data_key] == null or .data[$item.data_key] == "" then
                    .data[$item.data_key] = (
                        {}
                        | setpath_safe($item.path | split("."); $item.value)
                        | tojson
                    )
                else
                    .data[$item.data_key] |= (
                        fromjson
                        | setpath_safe($item.path | split("."); $item.value)
                        | tojson
                    )
                end
            )
        ' |
        kubectl apply -f -

    log_success "ConfigMap updated successfully"
}

# Create namespace if it does not exist (skip if already exists)
# Usage: create_or_skip_namespace <namespace>
create_or_skip_namespace() {
    local namespace="$1"

    if kubectl get namespace "$namespace" &>/dev/null; then
        log_info "Namespace '$namespace' already exists"
    else
        log_info "Creating namespace '$namespace'..."
        kubectl create namespace "$namespace"
        log_success "Namespace '$namespace' created"
    fi
}

# Check if required CLI tools exist
# Usage: check_cli_exist <tool1> [tool2] [tool3] ...
check_cli_exist() {
    local missing=()
    for cmd in "$@"; do
        if ! command_exists "$cmd"; then
            missing+=("$cmd")
        fi
    done

    if [ ${#missing[@]} -gt 0 ]; then
        log_error "Required CLI tool(s) not found: ${missing[*]}"
        log_error "Please install missing tool(s) first."
        exit 1
    fi
}

command_exists() {
    command -v "$1" &>/dev/null
}

# Retry a command with delay between attempts
# Usage: retry_command <max_attempts> <delay_seconds> <command...>
# Example: retry_command 3 5 kubectl apply -k "${RUNTIMES_DIR}"
# Returns: 0 on success, 1 on failure after all attempts
retry_command() {
    local max_attempts="$1"
    local delay="$2"
    shift 2
    local attempt=1

    while [ $attempt -le $max_attempts ]; do
        if "$@" 2>&1; then
            return 0
        fi

        if [ $attempt -lt $max_attempts ]; then
            log_warning "Command failed, retrying in ${delay} seconds... (attempt $attempt/$max_attempts)"
            sleep "$delay"
        else
            log_error "Command failed after $max_attempts attempts"
            return 1
        fi
        attempt=$((attempt + 1))
    done
}

# Compare semantic versions (returns 0 if v1 >= v2, 1 otherwise)
# Usage: version_gte "v3.17.3" "v3.16.0"
# Example: version_gte "$current_version" "$required_version" && echo "OK"
version_gte() {
    [ "$1" = "$(printf '%s\n' "$1" "$2" | sort -V | tail -1)" ]
}

# ============================================================================
# Shared Resources Configuration (for dual KServe + LLMISVC installation)
# ============================================================================

determine_shared_resources_config() {
    local install_mode="${1}"
    local enable_kserve="${2}"
    local enable_llmisvc="${3}"

    if ! is_positive "${enable_kserve}" && ! is_positive "${enable_llmisvc}"; then
        return
    fi

    log_info "Determining shared resources configuration (KSERVE=${enable_kserve}, LLMISVC=${enable_llmisvc})..."

    if [ "${install_mode}" = "helm" ]; then
        determine_shared_resources_helm "${enable_kserve}" "${enable_llmisvc}"
    elif [ "${install_mode}" = "kustomize" ]; then
        determine_shared_resources_kustomize
    else
        log_error "INSTALL_MODE not set. Must be 'helm' or 'kustomize'"
        exit 1
    fi
}

determine_shared_resources_helm() {
    local enable_kserve="${1}"
    local enable_llmisvc="${2}"

    local kserve_installed=$(helm list -n "${KSERVE_NAMESPACE}" -q 2>/dev/null | grep -c "^kserve-resources$" || true)
    local llmisvc_installed=$(helm list -n "${KSERVE_NAMESPACE}" -q 2>/dev/null | grep -c "^kserve-llmisvc-resources$" || true)

    if [ "${kserve_installed}" = "0" ] && [ "${llmisvc_installed}" = "0" ]; then
        # First installation - check which components are being enabled
        if is_positive "${enable_kserve}" && is_positive "${enable_llmisvc}"; then
            # Both enabled: kserve-resources installs first and creates shared resources
            log_info "[Helm] First install (both) - kserve-resources creates shared resources, llmisvc-resources does not"
            LLMISVC_EXTRA_ARGS="${LLMISVC_EXTRA_ARGS:-} --set kserve.createSharedResources=false"
        elif is_positive "${enable_kserve}" && ! is_positive "${enable_llmisvc}"; then
            # Only kserve enabled: kserve-resources creates shared resources
            log_info "[Helm] First install (kserve only) - kserve-resources will create shared resources"
            # Use default value (true) - no extra args needed
        elif ! is_positive "${enable_kserve}" && is_positive "${enable_llmisvc}"; then
            # Only llmisvc enabled: llmisvc-resources creates shared resources
            log_info "[Helm] First install (llmisvc only) - llmisvc-resources will create shared resources"
            # Use default value (true) - no extra args needed
        else
            # Neither enabled - shouldn't reach here
            log_error "[Helm] No components enabled"
            return 1
        fi
    elif [ "${kserve_installed}" = "1" ] && [ "${llmisvc_installed}" = "0" ]; then
        log_info "[Helm] Only kserve-resources installed - setting createSharedResources=false for LLMISVC"
        LLMISVC_EXTRA_ARGS="${LLMISVC_EXTRA_ARGS:-} --set kserve.createSharedResources=false"
    elif [ "${kserve_installed}" = "0" ] && [ "${llmisvc_installed}" = "1" ]; then
        log_info "[Helm] Only kserve-llmisvc-resources installed - setting createSharedResources=false for KSERVE"
        KSERVE_EXTRA_ARGS="${KSERVE_EXTRA_ARGS:-} --set kserve.createSharedResources=false"
    else
        local kserve_has_false=$(helm get values kserve-resources -n "${KSERVE_NAMESPACE}" 2>/dev/null | grep -c "createSharedResources: false" || true)

        if [ "${kserve_has_false}" = "1" ]; then
            log_info "[Helm] Maintaining createSharedResources=false for KSERVE"
            KSERVE_EXTRA_ARGS="${KSERVE_EXTRA_ARGS:-} --set kserve.createSharedResources=false"
        else
            log_info "[Helm] Setting createSharedResources=false for LLMISVC"
            LLMISVC_EXTRA_ARGS="${LLMISVC_EXTRA_ARGS:-} --set kserve.createSharedResources=false"
        fi
    fi
}

determine_shared_resources_kustomize() {
    KSERVE_INSTALLED=$(kubectl get deployment kserve-controller-manager -n "${KSERVE_NAMESPACE}" 2>/dev/null | grep -c "kserve-controller-manager" || true)
    LLMISVC_INSTALLED=$(kubectl get deployment llmisvc-controller-manager -n "${KSERVE_NAMESPACE}" 2>/dev/null | grep -c "llmisvc-controller-manager" || true)
    
    export KSERVE_INSTALLED
    export LLMISVC_INSTALLED

    log_info "[Kustomize] Installation status(0: not installed, 1: installed): KSERVE=${KSERVE_INSTALLED}, LLMISVC=${LLMISVC_INSTALLED}"
}

# ============================================================================

# Set environment variable based on priority order:
# Priority: 1. Runtime env > 2. Component env > 3. Global env > 4. Component default
# Usage: set_env_with_priority VAR_NAME COMPONENT_ENV_VALUE GLOBAL_ENV_VALUE DEFAULT_VALUE
set_env_with_priority() {
    local var_name="$1"
    local component_value="$2"
    local global_value="$3"
    local default_value="$4"

    # Get current value
    local current_value
    eval "current_value=\${${var_name}}"

    # If current value exists and differs from default, it's a runtime value - keep it
    if [ -n "$current_value" ] && [ -n "$default_value" ] && [ "$current_value" != "$default_value" ]; then
        # This is a runtime value, keep it
        return
    fi

    # Apply priority: component env > global env > default
    if [ -n "$component_value" ]; then
        eval "export $var_name=\"$component_value\""
    elif [ -n "$global_value" ]; then
        eval "export $var_name=\"$global_value\""
    fi
    # If both are empty, variable keeps its default value
}

#================================================
# Determine repository root using find_repo_root
#================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-.}")" && pwd)"
REPO_ROOT="$(find_repo_root "${SCRIPT_DIR}" "true")"
export REPO_ROOT

# Set up BIN_DIR - use repo bin if it exists, otherwise use temp directory
if [[ -d "${REPO_ROOT}/bin" ]]; then
    export BIN_DIR="${REPO_ROOT}/bin"
else
    export BIN_DIR="$(mktemp -d)"
    log_info "Using temp BIN_DIR: ${BIN_DIR}"
fi

export PATH="${BIN_DIR}:${PATH}"

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

export REINSTALL
export UNINSTALL
export FORCE_UPGRADE

# RELEASE mode (from definition file)
RELEASE="false"
export RELEASE

#================================================
# Version Dependencies (from kserve-deps.env)
#================================================

GOLANGCI_LINT_VERSION=v2.9.0
CONTROLLER_TOOLS_VERSION=v0.19.0
ENVTEST_VERSION=release-0.19
YQ_VERSION=v4.52.1
HELM_VERSION=v3.16.3
KUSTOMIZE_VERSION=v5.8.1
HELM_DOCS_VERSION=v1.12.0
POETRY_VERSION=1.8.3
UV_VERSION=0.7.8
RUFF_VERSION=0.14.13
PINACT_VERSION=v3.9.0
KIND_VERSION=v0.30.0
OPERATOR_SDK_VERSION=v1.42.0
CERT_MANAGER_VERSION=v1.17.0
ENVOY_GATEWAY_VERSION=v1.6.3
ENVOY_AI_GATEWAY_VERSION=v0.5.0
KNATIVE_OPERATOR_VERSION=v1.21.1
KNATIVE_SERVING_VERSION=1.21.1
KEDA_OTEL_ADDON_VERSION=v0.0.6
PROMETHEUS_VERSION=83.4.0
PROMETHEUS_ADAPTER_VERSION=5.3.0
KSERVE_VERSION=v0.18.0-rc1
ISTIO_VERSION=1.27.1
KEDA_VERSION=2.17.3
OPENTELEMETRY_OPERATOR_VERSION=0.74.3
LWS_VERSION=v0.8.0
GATEWAY_API_VERSION=v1.4.1
GIE_VERSION=v1.3.1
WVA_VERSION=v0.6.0

#================================================
# Global Variables (from global-vars.env)
#================================================
# These provide default namespace values that can be overridden
# by environment variables or GLOBAL_ENV settings below

KEDA_NAMESPACE="${KEDA_NAMESPACE:-keda}"
KSERVE_NAMESPACE="${KSERVE_NAMESPACE:-kserve}"
PROMETHEUS_NAMESPACE="${PROMETHEUS_NAMESPACE:-monitoring}"
PROMETHEUS_ADAPTER_NAMESPACE="${PROMETHEUS_ADAPTER_NAMESPACE:-monitoring}"
WVA_NAMESPACE="${WVA_NAMESPACE:-wva-system}"
OTEL_NAMESPACE="${OTEL_NAMESPACE:-opentelemetry-operator}"
OPERATOR_NAMESPACE="${OPERATOR_NAMESPACE:-knative-operator}"
SERVING_NAMESPACE="${SERVING_NAMESPACE:-knative-serving}"
ISTIO_NAMESPACE="${ISTIO_NAMESPACE:-istio-system}"
GATEWAY_NAMESPACE="${GATEWAY_NAMESPACE:-kserve}"
DEPLOYMENT_MODE="${DEPLOYMENT_MODE:-Knative}"
GATEWAY_NETWORK_LAYER="${GATEWAY_NETWORK_LAYER:-false}"
ENABLE_KSERVE="${ENABLE_KSERVE:-true}"
ENABLE_LLMISVC="${ENABLE_LLMISVC:-false}"
ENABLE_LOCALMODEL="${ENABLE_LOCALMODEL:-false}"
EMBED_MANIFESTS="${EMBED_MANIFESTS:-false}"
EMBED_TEMPLATES="${EMBED_TEMPLATES:-false}"
KSERVE_CUSTOM_ISVC_CONFIGS="${KSERVE_CUSTOM_ISVC_CONFIGS:-}"

#================================================
# Component-Specific Variables
#================================================

PROMETHEUS_NAMESPACE="${PROMETHEUS_NAMESPACE:-monitoring}"
PROMETHEUS_RELEASE_NAME="${PROMETHEUS_RELEASE_NAME:-prometheus}"
PROMETHEUS_ADAPTER_NAMESPACE="${PROMETHEUS_ADAPTER_NAMESPACE:-monitoring}"
PROMETHEUS_URL="${PROMETHEUS_URL:-https://prometheus-kube-prometheus-prometheus.monitoring}"
WVA_NAMESPACE="${WVA_NAMESPACE:-wva-system}"
WVA_RELEASE_NAME="${WVA_RELEASE_NAME:-llm-d-wva}"
WVA_PROMETHEUS_URL="${WVA_PROMETHEUS_URL:-https://prometheus-kube-prometheus-prometheus.monitoring:9090}"

#================================================
# Template Functions (EMBED_TEMPLATES MODE)
#================================================



#================================================
# Component Functions
#================================================

# ----------------------------------------
# CLI/Component: helm
# ----------------------------------------



install_helm() {
    local os=$(detect_os)
    local arch=$(detect_arch)
    local archive_name="helm-${HELM_VERSION}-${os}-${arch}.tar.gz"
    local download_url="https://get.helm.sh/${archive_name}"

    log_info "Installing Helm ${HELM_VERSION} for ${os}/${arch}..."

    # Check if helm is already installed in BIN_DIR with the exact required version
    if [[ -f "${BIN_DIR}/helm" ]]; then
        local current_version=$("${BIN_DIR}/helm" version --template='{{.Version}}' 2>/dev/null)
        if [[ "$current_version" == "$HELM_VERSION" ]]; then
            log_info "Helm ${current_version} is already installed in ${BIN_DIR}"
            return 0
        fi
        [[ -n "$current_version" ]] && log_info "Replacing Helm ${current_version} with ${HELM_VERSION} in ${BIN_DIR}..."
    fi

    local temp_dir=$(mktemp -d)
    local temp_file="${temp_dir}/${archive_name}"

    if command -v wget &>/dev/null; then
        wget -q "${download_url}" -O "${temp_file}"
    elif command -v curl &>/dev/null; then
        curl -sL "${download_url}" -o "${temp_file}"
    else
        log_error "Neither wget nor curl is available" >&2
        rm -rf "${temp_dir}"
        exit 1
    fi

    tar -xzf "${temp_file}" -C "${temp_dir}"

    local binary_path="${temp_dir}/${os}-${arch}/helm"

    if [[ ! -f "${binary_path}" ]]; then
        log_error "helm binary not found in archive" >&2
        rm -rf "${temp_dir}"
        exit 1
    fi

    chmod +x "${binary_path}"

    if [[ -w "${BIN_DIR}" ]]; then
        mv "${binary_path}" "${BIN_DIR}/helm"
    else
        sudo mv "${binary_path}" "${BIN_DIR}/helm"
    fi

    rm -rf "${temp_dir}"

    log_success "Successfully installed Helm ${HELM_VERSION} to ${BIN_DIR}/helm"
    "${BIN_DIR}/helm" version
}

# ----------------------------------------
# CLI/Component: prometheus-helm
# ----------------------------------------

uninstall_prometheus_helm() {
    log_info "Uninstalling Prometheus..."

    helm uninstall "${PROMETHEUS_RELEASE_NAME}" -n "${PROMETHEUS_NAMESPACE}" 2>/dev/null || true
    kubectl delete all --all -n "${PROMETHEUS_NAMESPACE}" --force --grace-period=0 2>/dev/null || true
    kubectl delete namespace "${PROMETHEUS_NAMESPACE}" --wait=true --timeout=60s --force --grace-period=0 2>/dev/null || true

    log_success "Prometheus uninstalled"
}

install_prometheus_helm() {
    if helm list -n "${PROMETHEUS_NAMESPACE}" 2>/dev/null | grep -q "${PROMETHEUS_RELEASE_NAME}"; then
        if [ "$REINSTALL" = false ]; then
            log_info "Prometheus is already installed. Use --reinstall to reinstall."
            return 0
        else
            log_info "Reinstalling Prometheus..."
            uninstall_prometheus_helm
        fi
    fi

    log_info "Adding prometheus-community Helm repository..."
    helm repo add prometheus-community https://prometheus-community.github.io/helm-charts --force-update

    log_info "Creating self-signed TLS certificate for Prometheus via cert-manager..."
    kubectl create namespace "${PROMETHEUS_NAMESPACE}" 2>/dev/null || true

    kubectl apply -f - <<EOF
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: prometheus-selfsigned
  namespace: ${PROMETHEUS_NAMESPACE}
spec:
  selfSigned: {}
---
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: prometheus-tls
  namespace: ${PROMETHEUS_NAMESPACE}
spec:
  secretName: prometheus-tls
  duration: 8760h
  issuerRef:
    name: prometheus-selfsigned
    kind: Issuer
  dnsNames:
    - ${PROMETHEUS_RELEASE_NAME}-kube-prometheus-prometheus.${PROMETHEUS_NAMESPACE}
    - ${PROMETHEUS_RELEASE_NAME}-kube-prometheus-prometheus.${PROMETHEUS_NAMESPACE}.svc
    - ${PROMETHEUS_RELEASE_NAME}-kube-prometheus-prometheus.${PROMETHEUS_NAMESPACE}.svc.cluster.local
EOF

    kubectl wait certificate prometheus-tls -n "${PROMETHEUS_NAMESPACE}" --for=condition=Ready --timeout=60s

    log_info "Installing kube-prometheus-stack ${PROMETHEUS_VERSION}..."
    helm install "${PROMETHEUS_RELEASE_NAME}" prometheus-community/kube-prometheus-stack \
        --namespace "${PROMETHEUS_NAMESPACE}" \
        --create-namespace \
        --version "${PROMETHEUS_VERSION}" \
        --set grafana.enabled=false \
        --set alertmanager.enabled=false \
        --set nodeExporter.enabled=false \
        --set kubeStateMetrics.enabled=false \
        --set prometheus.prometheusSpec.resources.requests.cpu=50m \
        --set prometheus.prometheusSpec.resources.requests.memory=256Mi \
        --set prometheus.prometheusSpec.resources.limits.cpu=200m \
        --set prometheus.prometheusSpec.resources.limits.memory=512Mi \
        --set prometheus.prometheusSpec.web.tlsConfig.cert.secret.name=prometheus-tls \
        --set prometheus.prometheusSpec.web.tlsConfig.cert.secret.key=tls.crt \
        --set prometheus.prometheusSpec.web.tlsConfig.keySecret.name=prometheus-tls \
        --set prometheus.prometheusSpec.web.tlsConfig.keySecret.key=tls.key \
        --wait \
        --timeout 10m \
        ${PROMETHEUS_EXTRA_ARGS:-}

    log_success "Successfully installed kube-prometheus-stack ${PROMETHEUS_VERSION} via Helm"

    wait_for_pods "${PROMETHEUS_NAMESPACE}" "app.kubernetes.io/name=prometheus" "300s"

    log_success "Prometheus is ready (TLS enabled)!"
}

# ----------------------------------------
# CLI/Component: prometheus-adapter-helm
# ----------------------------------------

uninstall_prometheus_adapter_helm() {
    log_info "Uninstalling Prometheus Adapter..."

    helm uninstall prometheus-adapter -n "${PROMETHEUS_ADAPTER_NAMESPACE}" 2>/dev/null || true

    log_success "Prometheus Adapter uninstalled"
}

install_prometheus_adapter_helm() {
    if helm list -n "${PROMETHEUS_ADAPTER_NAMESPACE}" 2>/dev/null | grep -q "prometheus-adapter"; then
        if [ "$REINSTALL" = false ]; then
            log_info "Prometheus Adapter is already installed. Use --reinstall to reinstall."
            return 0
        else
            log_info "Reinstalling Prometheus Adapter..."
            uninstall_prometheus_adapter_helm
        fi
    fi

    log_info "Adding prometheus-community Helm repository..."
    helm repo add prometheus-community https://prometheus-community.github.io/helm-charts --force-update

    log_info "Installing Prometheus Adapter ${PROMETHEUS_ADAPTER_VERSION}..."
    helm install prometheus-adapter prometheus-community/prometheus-adapter \
        --namespace "${PROMETHEUS_ADAPTER_NAMESPACE}" \
        --create-namespace \
        --version "${PROMETHEUS_ADAPTER_VERSION}" \
        --set prometheus.url="${PROMETHEUS_URL}" \
        --set prometheus.port=9090 \
        --set certManager.enabled=true \
        --set extraArguments[0]="--prometheus-ca-file=/etc/prometheus-tls/ca.crt" \
        --set extraVolumes[0].name=prometheus-tls \
        --set extraVolumes[0].secret.secretName=prometheus-tls \
        --set extraVolumeMounts[0].name=prometheus-tls \
        --set extraVolumeMounts[0].mountPath=/etc/prometheus-tls \
        --set extraVolumeMounts[0].readOnly=true \
        --set rules.external[0].seriesQuery='wva_desired_replicas' \
        --set 'rules.external[0].resources.overrides.exported_namespace.resource=namespace' \
        --set 'rules.external[0].name.matches=^(.*)' \
        --set 'rules.external[0].name.as=${1}' \
        --set 'rules.external[0].metricsQuery=<<.Series>>{<<.LabelMatchers>>}' \
        --wait \
        --timeout 10m \
        ${PROMETHEUS_ADAPTER_EXTRA_ARGS:-}

    log_success "Successfully installed Prometheus Adapter ${PROMETHEUS_ADAPTER_VERSION} via Helm"

    wait_for_pods "${PROMETHEUS_ADAPTER_NAMESPACE}" "app.kubernetes.io/name=prometheus-adapter" "300s"

    log_success "Prometheus Adapter is ready!"
}

# ----------------------------------------
# CLI/Component: wva-helm
# ----------------------------------------

uninstall_wva_helm() {
    log_info "Uninstalling WVA..."

    helm uninstall "${WVA_RELEASE_NAME}" -n "${WVA_NAMESPACE}" 2>/dev/null || true
    kubectl delete all --all -n "${WVA_NAMESPACE}" --force --grace-period=0 2>/dev/null || true
    kubectl delete namespace "${WVA_NAMESPACE}" --wait=true --timeout=60s --force --grace-period=0 2>/dev/null || true

    log_success "WVA uninstalled"
}

install_wva_helm() {
    if helm list -n "${WVA_NAMESPACE}" 2>/dev/null | grep -q "${WVA_RELEASE_NAME}"; then
        if [ "$REINSTALL" = false ]; then
            log_info "WVA is already installed. Use --reinstall to reinstall."
            return 0
        else
            log_info "Reinstalling WVA..."
            uninstall_wva_helm
        fi
    fi

    # Strip leading 'v' from WVA_VERSION if present (chart uses semver without prefix)
    local chart_version="${WVA_VERSION#v}"

    log_info "Installing WVA ${chart_version}..."
    helm install "${WVA_RELEASE_NAME}" oci://ghcr.io/llm-d/workload-variant-autoscaler \
        --namespace "${WVA_NAMESPACE}" \
        --create-namespace \
        --version "${chart_version}" \
        --set controller.enabled=true \
        --set wva.enabled=true \
        --set wva.replicaCount=1 \
        --set wva.namespaceScoped=false \
        --set wva.prometheus.baseURL="${WVA_PROMETHEUS_URL}" \
        --set wva.prometheus.tls.insecureSkipVerify=true \
        --set wva.prometheus.serviceAccountName="prometheus-kube-prometheus-prometheus" \
        --set wva.prometheus.monitoringNamespace="${PROMETHEUS_NAMESPACE:-monitoring}" \
        --set wva.metrics.secure=false \
        --set va.enabled=false \
        --set hpa.enabled=false \
        --set vllmService.enabled=false \
        --set llmd.namespace="${WVA_NAMESPACE}" \
        --wait \
        --timeout 5m \
        ${WVA_EXTRA_ARGS:-}

    log_success "Successfully installed WVA ${chart_version} via Helm"

    wait_for_pods "${WVA_NAMESPACE}" "control-plane=controller-manager" "300s"

    log_success "WVA is ready!"
}



#================================================
# Main Installation Logic
#================================================

main() {
    if [ "$UNINSTALL" = true ]; then
        echo "=========================================="
        echo "Uninstalling components..."
        echo "=========================================="
        uninstall_wva_helm
        uninstall_prometheus_adapter_helm
        uninstall_prometheus_helm
        
        echo "=========================================="
        echo "✅ Uninstallation completed!"
        echo "=========================================="
        exit 0
    fi

    echo "=========================================="
    echo "Install KServe LLM InferenceService HPA autoscaling dependencies (Prometheus, Prometheus Adapter, WVA)"
    echo "=========================================="

    export EMBED_TEMPLATES="true"

    install_helm
    install_prometheus_helm
    install_prometheus_adapter_helm
    install_wva_helm

    echo "=========================================="
    echo "✅ Installation completed successfully!"
    echo "=========================================="
}



main "$@"
