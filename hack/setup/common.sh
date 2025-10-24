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

set -o errexit
set -o nounset
set -o pipefail

# ============================================================================
# Utility Functions
# ============================================================================

find_repo_root() {
    local current_dir="${1:-$(pwd)}"

    while [[ "$current_dir" != "/" ]]; do
        if [[ -d "${current_dir}/.git" ]]; then
            echo "$current_dir"
            return 0
        fi
        current_dir="$(dirname "$current_dir")"
    done

    echo "Error: Could not find git repository root" >&2
    exit 1
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
        *)       echo "Unsupported OS" >&2; exit 1 ;;
    esac
    echo "$os"
}

detect_arch() {
    local arch=""
    case "$(uname -m)" in
        x86_64)  arch="amd64" ;;
        aarch64|arm64) arch="arm64" ;;
        *)       echo "Unsupported architecture" >&2; exit 1 ;;
    esac
    echo "$arch"
}

log_info() {
    echo "[INFO] $*"
}

log_error() {
    echo "[ERROR] $*" >&2
}

log_success() {
    echo "[SUCCESS] $*"
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
        local pod_count=$(kubectl get pods -n "$namespace" -l "$label_selector" --no-headers 2>/dev/null | wc -l)

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
    kubectl wait --for=condition=Ready pod -l "$label_selector" -n "$namespace" --timeout="$timeout"
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
# Auto-initialization (runs when this file is sourced)
# ============================================================================

# Auto-detect and export REPO_ROOT/BIN_DIR/PATH when sourced
if [[ -z "${REPO_ROOT:-}" ]]; then
    REPO_ROOT="$(find_repo_root "$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")")"
    export REPO_ROOT
    export BIN_DIR="${REPO_ROOT}/bin"
    ensure_dir "${BIN_DIR}"
    export PATH="${BIN_DIR}:${PATH}"
fi

# Load version dependencies
KSERVE_DEPS_FILE="${REPO_ROOT}/kserve-deps.env"
if [[ -f "${KSERVE_DEPS_FILE}" ]]; then
    source "${KSERVE_DEPS_FILE}"
fi

# Load global variables
GLOBAL_VARS_FILE="${REPO_ROOT}/hack/setup/global-vars.env"
if [[ -f "${GLOBAL_VARS_FILE}" ]]; then
    source "${GLOBAL_VARS_FILE}"
fi
