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

# Setup local Kind cluster with registry for KServe development
# Usage: manage.kind-with-registry.sh [--reinstall|--uninstall]
#   or:  REINSTALL=true manage.kind-with-registry.sh
#   or:  UNINSTALL=true manage.kind-with-registry.sh

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

# Registry settings
KIND_CLUSTER_NAME="${KIND_CLUSTER_NAME:-kind}"
KIND_REGISTRY_NAME="${KIND_REGISTRY_NAME:-kind-registry}"
KIND_REGISTRY_PORT="${KIND_REGISTRY_PORT:-5001}"

# Install kind if not present
if ! command_exists kind; then
    log_warning "kind not found, installing..."
    "${SCRIPT_DIR}/../cli/install-kind.sh"
fi

check_cli_exist kind docker kubectl

uninstall() {
    log_info "Destroying Kind cluster and registry..."
    kind delete cluster --name "${KIND_CLUSTER_NAME}" 2>/dev/null || true
    docker stop "${KIND_REGISTRY_NAME}" 2>/dev/null || true
    docker rm "${KIND_REGISTRY_NAME}" 2>/dev/null || true
    log_success "Kind cluster and registry destroyed"
}

install() {
    # Check if cluster already exists
    if kind get clusters 2>/dev/null | grep -q "^${KIND_CLUSTER_NAME}$"; then
        if [ "$REINSTALL" = false ]; then
            log_info "Kind cluster '${KIND_CLUSTER_NAME}' already exists. Use --reinstall to recreate."
            return 0
        else
            log_info "Recreating Kind cluster..."
            uninstall
        fi
    fi

    # Create registry
    log_info "Creating local registry '${KIND_REGISTRY_NAME}'..."
    if [ "$(docker inspect -f '{{.State.Running}}' "${KIND_REGISTRY_NAME}" 2>/dev/null || true)" != 'true' ]; then
        docker run -d --restart=always \
            -p "127.0.0.1:${KIND_REGISTRY_PORT}:5000" \
            --name "${KIND_REGISTRY_NAME}" \
            registry:2
        log_success "Registry created at localhost:${KIND_REGISTRY_PORT}"
    else
        log_info "Registry already running at localhost:${KIND_REGISTRY_PORT}"
    fi

    # Create Kind cluster
    log_info "Creating Kind cluster '${KIND_CLUSTER_NAME}'..."
    cat <<EOF | kind create cluster --name "${KIND_CLUSTER_NAME}" --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
containerdConfigPatches:
- |-
  [plugins."io.containerd.grpc.v1.cri".registry]
    config_path = "/etc/containerd/certs.d"
nodes:
  - role: control-plane
  - role: worker
EOF

    log_success "Kind cluster created"

    # Connect registry to kind network
    log_info "Connecting registry to kind network..."
    docker network connect "kind" "${KIND_REGISTRY_NAME}" 2>/dev/null || true

    # Configure containerd on each node
    log_info "Configuring containerd on nodes..."
    for node in $(kind get nodes --name "${KIND_CLUSTER_NAME}"); do
        docker exec "${node}" mkdir -p "/etc/containerd/certs.d/localhost:${KIND_REGISTRY_PORT}"
        cat <<EOF | docker exec -i "${node}" sh -c "cat > /etc/containerd/certs.d/localhost:${KIND_REGISTRY_PORT}/hosts.toml"
[host."http://${KIND_REGISTRY_NAME}:5000"]
EOF
    done

    # Document the local registry
    log_info "Documenting local registry..."
    cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: ConfigMap
metadata:
  name: local-registry-hosting
  namespace: kube-public
data:
  localRegistryHosting.v1: |
    host: "localhost:${KIND_REGISTRY_PORT}"
    help: "https://kind.sigs.k8s.io/docs/user/local-registry/"
EOF

    log_success "Kind cluster '${KIND_CLUSTER_NAME}' is ready with registry at localhost:${KIND_REGISTRY_PORT}"
    echo ""
    log_info "Next steps:"
    echo -e "  ${GREEN}export KO_DOCKER_REPO=localhost:${KIND_REGISTRY_PORT}${RESET}"
    echo -e "  ${GREEN}make deploy-dev${RESET}"
    echo ""
    log_info "Quick redeploy after code changes (~2 minutes):"
    echo -e "  ${GREEN}make redeploy-dev-image${RESET}  # Rebuilds images and updates deployments"
}

if [ "$UNINSTALL" = true ]; then
    uninstall
    exit 0
fi

install
