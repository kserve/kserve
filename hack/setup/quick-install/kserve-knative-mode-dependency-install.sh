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

# Install KServe Knative Mode dependencies only
#
# AUTO-GENERATED from: kserve-knative-mode-dependency-install.definition
# DO NOT EDIT MANUALLY
#
# To regenerate:
#   ./scripts/generate-install-script.py kserve-knative-mode-dependency-install.definition
#
# Usage: kserve-knative-mode-dependency-install.sh [--reinstall|--uninstall]

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
        if [[ -d "${current_dir}/.git" ]]; then
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
                    .
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

# Compare semantic versions (returns 0 if v1 >= v2, 1 otherwise)
# Usage: version_gte "v3.17.3" "v3.16.0"
# Example: version_gte "$current_version" "$required_version" && echo "OK"
version_gte() {
    [ "$1" = "$(printf '%s\n' "$1" "$2" | sort -V | tail -1)" ]
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

    # If current value differs from default/component/global, it must be runtime - keep it
    if [ -n "$current_value" ] && [ "$current_value" != "$default_value" ] &&
       [ "$current_value" != "$component_value" ] && [ "$current_value" != "$global_value" ]; then
        # This is a runtime value, keep it
        return
    fi

    # Apply priority: component env > global env > default
    if [ -n "$component_value" ]; then
        export "$var_name=$component_value"
    elif [ -n "$global_value" ]; then
        export "$var_name=$global_value"
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

UNINSTALL="${UNINSTALL:-false}"
REINSTALL="${REINSTALL:-false}"

if [[ "$*" == *"--uninstall"* ]]; then
    UNINSTALL=true
elif [[ "$*" == *"--reinstall"* ]]; then
    REINSTALL=true
fi

export REINSTALL
export UNINSTALL

# RELEASE mode (from definition file)
RELEASE="false"
export RELEASE

#================================================
# Version Dependencies (from kserve-deps.env)
#================================================

GOLANGCI_LINT_VERSION=v1.64.8
CONTROLLER_TOOLS_VERSION=v0.19.0
ENVTEST_VERSION=latest
YQ_VERSION=v4.52.1
HELM_VERSION=v3.16.3
KUSTOMIZE_VERSION=v5.5.0
HELM_DOCS_VERSION=v1.12.0
BLACK_FMT_VERSION=24.3
POETRY_VERSION=1.8.3
UV_VERSION=0.7.8
RUFF_VERSION=0.14.13
KIND_VERSION=v0.30.0
CERT_MANAGER_VERSION=v1.17.0
ENVOY_GATEWAY_VERSION=v1.5.0
ENVOY_AI_GATEWAY_VERSION=v0.4.0
KNATIVE_OPERATOR_VERSION=v1.16.0
KNATIVE_SERVING_VERSION=1.15.2
KEDA_OTEL_ADDON_VERSION=v0.0.6
KSERVE_VERSION=v0.16.0
ISTIO_VERSION=1.27.1
KEDA_VERSION=2.17.2
OPENTELEMETRY_OPERATOR_VERSION=0.74.3
LWS_VERSION=v0.7.0
GATEWAY_API_VERSION=v1.3.0
GIE_VERSION=v1.2.0

#================================================
# Global Variables (from global-vars.env)
#================================================
# These provide default namespace values that can be overridden
# by environment variables or GLOBAL_ENV settings below

KEDA_NAMESPACE="${KEDA_NAMESPACE:-keda}"
KSERVE_NAMESPACE="${KSERVE_NAMESPACE:-kserve}"
OTEL_NAMESPACE="${OTEL_NAMESPACE:-opentelemetry-operator}"
OPERATOR_NAMESPACE="${OPERATOR_NAMESPACE:-knative-operator}"
SERVING_NAMESPACE="${SERVING_NAMESPACE:-knative-serving}"
ISTIO_NAMESPACE="${ISTIO_NAMESPACE:-istio-system}"
GATEWAY_NAMESPACE="${GATEWAY_NAMESPACE:-kserve}"
DEPLOYMENT_MODE="${DEPLOYMENT_MODE:-Knative}"
GATEWAY_NETWORK_LAYER="${GATEWAY_NETWORK_LAYER:-false}"
LLMISVC="${LLMISVC:-false}"
EMBED_MANIFESTS="${EMBED_MANIFESTS:-false}"
KSERVE_CUSTOM_ISVC_CONFIGS="${KSERVE_CUSTOM_ISVC_CONFIGS:-}"

#================================================
# Component-Specific Variables
#================================================

ADDON_RELEASE_NAME="keda-otel-scaler"
OTEL_RELEASE_NAME="my-opentelemetry-operator"

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
    helm version
}

# ----------------------------------------
# CLI/Component: kustomize
# ----------------------------------------



install_kustomize() {
    local os=$(detect_os)
    local arch=$(detect_arch)
    local archive_name="kustomize_${KUSTOMIZE_VERSION}_${os}_${arch}.tar.gz"
    local download_url="https://github.com/kubernetes-sigs/kustomize/releases/download/kustomize%2F${KUSTOMIZE_VERSION}/${archive_name}"

    log_info "Installing Kustomize ${KUSTOMIZE_VERSION} for ${os}/${arch}..."

    if command -v kustomize &>/dev/null; then
        local current_version=$(kustomize version --short 2>/dev/null | grep -oP 'v[0-9.]+')
        if [[ -n "$current_version" ]] && version_gte "$current_version" "$KUSTOMIZE_VERSION"; then
            log_info "Kustomize ${current_version} is already installed (>= ${KUSTOMIZE_VERSION})"
            return 0
        fi
        [[ -n "$current_version" ]] && log_info "Upgrading Kustomize from ${current_version} to ${KUSTOMIZE_VERSION}..."
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

    local binary_path="${temp_dir}/kustomize"

    if [[ ! -f "${binary_path}" ]]; then
        log_error "kustomize binary not found in archive" >&2
        rm -rf "${temp_dir}"
        exit 1
    fi

    chmod +x "${binary_path}"

    if [[ -w "${BIN_DIR}" ]]; then
        mv "${binary_path}" "${BIN_DIR}/kustomize"
    else
        sudo mv "${binary_path}" "${BIN_DIR}/kustomize"
    fi

    rm -rf "${temp_dir}"

    log_success "Successfully installed Kustomize ${KUSTOMIZE_VERSION} to ${BIN_DIR}/kustomize"
    kustomize version
}

# ----------------------------------------
# CLI/Component: yq
# ----------------------------------------



install_yq() {
    local os=$(detect_os)
    local arch=$(detect_arch)
    local binary_name="yq_${os}_${arch}"
    local download_url="https://github.com/mikefarah/yq/releases/download/${YQ_VERSION}/${binary_name}"

    log_info "Installing yq ${YQ_VERSION} for ${os}/${arch}..."

    if [[ -x "${BIN_DIR}/yq" ]]; then
        local current_version=$("${BIN_DIR}/yq" --version 2>&1 | grep -oP 'version \K[v0-9.]+')
        # Normalize version format (add 'v' prefix if missing)
        [[ -n "$current_version" && "$current_version" != v* ]] && current_version="v${current_version}"
        if [[ -n "$current_version" ]] && version_gte "$current_version" "$YQ_VERSION"; then
            log_info "yq ${current_version} is already installed in ${BIN_DIR} (>= ${YQ_VERSION})"
            return 0
        fi
        [[ -n "$current_version" ]] && log_info "Upgrading yq from ${current_version} to ${YQ_VERSION}..."
    fi

    local temp_file=$(mktemp)

    if command -v wget &>/dev/null; then
        wget -q "${download_url}" -O "${temp_file}"
    elif command -v curl &>/dev/null; then
        curl -sL "${download_url}" -o "${temp_file}"
    else
        log_info "Neither wget nor curl is available" >&2
        rm -f "${temp_file}"
        exit 1
    fi

    chmod +x "${temp_file}"

    if [[ -w "${BIN_DIR}" ]]; then
        mv "${temp_file}" "${BIN_DIR}/yq"
    else
        sudo mv "${temp_file}" "${BIN_DIR}/yq"
    fi

    log_success "Successfully installed yq ${YQ_VERSION} to ${BIN_DIR}/yq"
    "${BIN_DIR}/yq" --version
}

# ----------------------------------------
# CLI/Component: cert-manager
# ----------------------------------------

uninstall_cert_manager() {
    log_info "Uninstalling cert-manager..."
    helm uninstall cert-manager -n cert-manager 2>/dev/null || true
    kubectl delete all --all -n cert-manager --force --grace-period=0 2>/dev/null || true
    kubectl delete namespace cert-manager --wait=true --timeout=60s --force --grace-period=0 2>/dev/null || true
    log_success "cert-manager uninstalled"
}

install_cert_manager() {
    if helm list -n cert-manager 2>/dev/null | grep -q "cert-manager"; then
        if [ "$REINSTALL" = false ]; then
            log_info "cert-manager is already installed. Use --reinstall to reinstall."
            return 0
        else
            log_info "Reinstalling cert-manager..."
            uninstall_cert_manager
        fi
    fi

    log_info "Adding cert-manager Helm repository..."
    helm repo add jetstack https://charts.jetstack.io --force-update

    log_info "Installing cert-manager ${CERT_MANAGER_VERSION}..."
    helm install \
        cert-manager jetstack/cert-manager \
        --namespace cert-manager \
        --create-namespace \
        --version "${CERT_MANAGER_VERSION}" \
        --set crds.enabled=true \
        --wait

    log_success "Successfully installed cert-manager ${CERT_MANAGER_VERSION} via Helm"

    wait_for_pods "cert-manager" "app in (cert-manager,webhook,cainjector)" "180s"

    log_success "cert-manager is ready!"
}

# ----------------------------------------
# CLI/Component: istio
# ----------------------------------------

uninstall_istio() {
    log_info "Uninstalling Istio..."
    helm uninstall istio-ingressgateway -n "${ISTIO_NAMESPACE}" 2>/dev/null || true
    helm uninstall istiod -n "${ISTIO_NAMESPACE}" 2>/dev/null || true
    helm uninstall istio-base -n "${ISTIO_NAMESPACE}" 2>/dev/null || true
    kubectl delete all --all -n "${ISTIO_NAMESPACE}" --force --grace-period=0 2>/dev/null || true
    kubectl delete namespace "${ISTIO_NAMESPACE}" --wait=true --timeout=60s --force --grace-period=0 2>/dev/null || true
    log_success "Istio uninstalled"
}

install_istio() {
    if helm list -n "${ISTIO_NAMESPACE}" 2>/dev/null | grep -q "istio-base"; then
        if [ "$REINSTALL" = false ]; then
            log_info "Istio is already installed. Use --reinstall to reinstall."
            return 0
        else
            log_info "Reinstalling Istio..."
            uninstall_istio
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

# ----------------------------------------
# CLI/Component: istio-ingress-class
# ----------------------------------------

uninstall_istio_ingress_class() {
    log_info "Deleting Istio IngressClass 'istio'..."
    kubectl delete ingressclass "istio" --ignore-not-found=true --force --grace-period=0 2>/dev/null || true
    log_success "Istio IngressClass 'istio' deleted"
}

install_istio_ingress_class() {
    if kubectl get ingressclass "istio" &>/dev/null; then
        if [ "$REINSTALL" = false ]; then
            log_info "Istio IngressClass 'istio' already exists. Use --reinstall to recreate."
            return 0
        else
            log_info "Recreating Istio IngressClass 'istio'..."
            uninstall_istio_ingress_class
        fi
    fi

    log_info "Creating Istio IngressClass 'istio'..."
    cat <<EOF | kubectl apply -f -
apiVersion: networking.k8s.io/v1
kind: IngressClass
metadata:
  name: istio
spec:
  controller: istio.io/ingress-controller
EOF

    log_success "Istio IngressClass 'istio' created successfully!"
}

# ----------------------------------------
# CLI/Component: keda
# ----------------------------------------

uninstall_keda() {
    log_info "Uninstalling KEDA..."

    helm uninstall keda-otel-scaler -n "${KEDA_NAMESPACE}" 2>/dev/null || true
    helm uninstall keda -n "${KEDA_NAMESPACE}" 2>/dev/null || true
    kubectl delete all --all -n "${KEDA_NAMESPACE}" --force --grace-period=0 2>/dev/null || true
    kubectl delete namespace "${KEDA_NAMESPACE}" --wait=true --timeout=60s --force --grace-period=0 2>/dev/null || true

    log_success "KEDA uninstalled"
}

install_keda() {
    if helm list -n "${KEDA_NAMESPACE}" 2>/dev/null | grep -q "keda"; then
        if [ "$REINSTALL" = false ]; then
            log_info "KEDA is already installed. Use --reinstall to reinstall."
            return 0
        else
            log_info "Reinstalling KEDA..."
            uninstall_keda
        fi
    fi

    log_info "Adding KEDA Helm repository..."
    helm repo add kedacore https://kedacore.github.io/charts --force-update

    log_info "Installing KEDA ${KEDA_VERSION}..."
    helm install keda kedacore/keda \
        --namespace "${KEDA_NAMESPACE}" \
        --create-namespace \
        --version "${KEDA_VERSION}" \
        --wait \
        ${KEDA_EXTRA_ARGS:-}

    log_success "Successfully installed KEDA ${KEDA_VERSION} via Helm"

    wait_for_pods "${KEDA_NAMESPACE}" "app.kubernetes.io/name=keda-operator" "300s"

    log_success "KEDA is ready!"
}

# ----------------------------------------
# CLI/Component: keda-otel-addon
# ----------------------------------------

uninstall_keda_otel_addon() {
    log_info "Uninstalling KEDA OTel add-on..."
    helm uninstall "${ADDON_RELEASE_NAME}" -n "${KEDA_NAMESPACE}" 2>/dev/null || true
    log_success "KEDA OTel add-on uninstalled"
}

install_keda_otel_addon() {
    if ! kubectl get namespace "${KEDA_NAMESPACE}" &>/dev/null; then
        log_error "KEDA namespace '${KEDA_NAMESPACE}' does not exist. Please install KEDA first."
        exit 1
    fi

    if helm list -n "${KEDA_NAMESPACE}" 2>/dev/null | grep -q "${ADDON_RELEASE_NAME}"; then
        if [ "$REINSTALL" = false ]; then
            log_info "KEDA OTel add-on is already installed. Use --reinstall to reinstall."
            return 0
        else
            log_info "Reinstalling KEDA OTel add-on..."
            uninstall_keda_otel_addon
        fi
    fi

    log_info "Installing KEDA OTel add-on ${KEDA_OTEL_ADDON_VERSION} from kedify/otel-add-on..."
    helm upgrade -i "${ADDON_RELEASE_NAME}" \
        oci://ghcr.io/kedify/charts/otel-add-on \
        --namespace "${KEDA_NAMESPACE}" \
        --version="${KEDA_OTEL_ADDON_VERSION}" \
        --wait \
        ${KEDA_OTEL_ADDON_EXTRA_ARGS:-}

    log_success "Successfully installed KEDA OTel add-on ${KEDA_OTEL_ADDON_VERSION} via Helm"

    wait_for_pods "${KEDA_NAMESPACE}" "app.kubernetes.io/instance=${ADDON_RELEASE_NAME}" "300s"

    log_success "KEDA OTel add-on is ready!"
}

# ----------------------------------------
# CLI/Component: opentelemetry
# ----------------------------------------

uninstall_opentelemetry() {
    log_info "Uninstalling OpenTelemetry Operator..."
    helm uninstall "${OTEL_RELEASE_NAME}" -n "${OTEL_NAMESPACE}" 2>/dev/null || true
    kubectl delete all --all -n "${OTEL_NAMESPACE}" --force --grace-period=0 2>/dev/null || true
    kubectl delete namespace "${OTEL_NAMESPACE}" --wait=true --timeout=60s --force --grace-period=0 2>/dev/null || true
    log_success "OpenTelemetry Operator uninstalled"
}

install_opentelemetry() {
    if helm list -n "${OTEL_NAMESPACE}" 2>/dev/null | grep -q "${OTEL_RELEASE_NAME}"; then
        if [ "$REINSTALL" = false ]; then
            log_info "OpenTelemetry Operator is already installed. Use --reinstall to reinstall."
            return 0
        else
            log_info "Reinstalling OpenTelemetry Operator..."
            uninstall_opentelemetry
        fi
    fi

    log_info "Adding OpenTelemetry Helm repository..."
    helm repo add open-telemetry https://open-telemetry.github.io/opentelemetry-helm-charts --force-update

    log_info "Installing OpenTelemetry Operator ${OPENTELEMETRY_OPERATOR_VERSION}..."
    helm install "${OTEL_RELEASE_NAME}" open-telemetry/opentelemetry-operator \
        --namespace "${OTEL_NAMESPACE}" \
        --create-namespace \
        --version "${OPENTELEMETRY_OPERATOR_VERSION}" \
        --wait \
        --set "manager.collectorImage.repository=otel/opentelemetry-collector-contrib" \
        ${OTEL_OPERATOR_EXTRA_ARGS:-}

    log_success "Successfully installed OpenTelemetry Operator via Helm"

    wait_for_pods "${OTEL_NAMESPACE}" "app.kubernetes.io/name=opentelemetry-operator" "300s"

    log_success "OpenTelemetry Operator is ready!"
}



#================================================
# Main Installation Logic
#================================================

main() {
    if [ "$UNINSTALL" = true ]; then
        echo "=========================================="
        echo "Uninstalling components..."
        echo "=========================================="
        uninstall_opentelemetry
        uninstall_keda_otel_addon
        uninstall_keda
        uninstall_istio_ingress_class
        uninstall_istio
        uninstall_cert_manager
        
        
        
        echo "=========================================="
        echo "✅ Uninstallation completed!"
        echo "=========================================="
        exit 0
    fi

    echo "=========================================="
    echo "Install KServe Knative Mode dependencies only"
    echo "=========================================="



    install_helm
    install_kustomize
    install_yq
    install_cert_manager
    install_istio
    install_istio_ingress_class
    install_keda
    install_keda_otel_addon
    install_opentelemetry

    echo "=========================================="
    echo "✅ Installation completed successfully!"
    echo "=========================================="
}



main "$@"
