#!/usr/bin/env bash
#
# KServe odh-xks Setup Script for KinD
#
# This script provisions a complete KinD-based test environment for the
# odh-xks overlay with Istio 1.27, cert-manager, LWS, and Gateway API.
#
# Usage:
#   ./test/scripts/kind/setup-odh-xks.sh
#
# Environment variables (with defaults):
#   KIND_CLUSTER_NAME=kind
#   KUBERNETES_VERSION=v1.32.0
#   ISTIO_VERSION=1.27.5
#   CERT_MANAGER_VERSION=v1.16.1
#   LWS_VERSION=v0.6.2
#   GATEWAY_API_VERSION=v1.2.1
#   KSERVE_NAMESPACE=opendatahub
#   KO_DOCKER_REPO=local
#   LLMISVC_CONTROLLER_IMG=llmisvc-controller:dev
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -euo pipefail

# Configuration Variables
KIND_CLUSTER_NAME="${KIND_CLUSTER_NAME:-kind}"
KUBERNETES_VERSION="${KUBERNETES_VERSION:-v1.32.0}"
ISTIO_VERSION="${ISTIO_VERSION:-1.27.5}"
CERT_MANAGER_VERSION="${CERT_MANAGER_VERSION:-v1.16.1}"
LWS_VERSION="${LWS_VERSION:-v0.6.2}"
GATEWAY_API_VERSION="${GATEWAY_API_VERSION:-v1.4.1}"
KSERVE_NAMESPACE="${KSERVE_NAMESPACE:-opendatahub}"
KO_DOCKER_REPO="${KO_DOCKER_REPO:-local}"
LLMISVC_CONTROLLER_IMG="${LLMISVC_CONTROLLER_IMG:-llmisvc-controller:dev}"
KSERVE_CONTROLLER_IMAGE="${KO_DOCKER_REPO}/${LLMISVC_CONTROLLER_IMG}"

# Determine script and project directories
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd)"

# -----------------------------------------------------------------------------
# Helper Functions
# -----------------------------------------------------------------------------

log_info() {
  echo "ℹ️  $*"
}

log_success() {
  echo "✅ $*"
}

log_error() {
  echo "❌ $*" >&2
}

log_wait() {
  echo "⏳ $*"
}

# wait_for_crd <crd-name> [timeout]
wait_for_crd() {
  local crd="$1"
  local timeout="${2:-120s}"

  log_wait "Waiting for CRD ${crd} to appear (timeout: ${timeout})..."
  if ! timeout "$timeout" bash -c 'until kubectl get crd "$1" &>/dev/null; do sleep 2; done' _ "$crd"; then
    log_error "Timed out after $timeout waiting for CRD $crd to appear."
    return 1
  fi

  log_wait "CRD ${crd} detected — waiting for it to become Established..."
  # Wait for status.conditions to be populated before using kubectl wait
  timeout "$timeout" bash -c 'until kubectl get crd "$1" -o jsonpath="{.status.conditions}" 2>/dev/null | grep -q "Established"; do sleep 1; done' _ "$crd"
}

# wait_for_pods <namespace> <label-selector> [timeout]
wait_for_pods() {
  local ns="$1"
  local label="$2"
  local timeout="${3:-300s}"

  log_wait "Waiting for pods -l '$label' in namespace '$ns' to be ready (timeout: $timeout)..."

  # Wait for at least one pod to exist
  local max_attempts=60
  local attempt=0
  while ! kubectl get pod -n "$ns" -l "$label" -o name 2>/dev/null | grep -q .; do
    ((++attempt))
    if [[ $attempt -ge $max_attempts ]]; then
      log_error "Timed out waiting for pods -l '$label' to appear in namespace '$ns'"
      return 1
    fi
    sleep 2
  done

  kubectl wait --for=condition=ready pod -l "$label" -n "$ns" --timeout="$timeout"
}

# -----------------------------------------------------------------------------
# check_dependencies
# Verify kind, kubectl, kustomize are available
# -----------------------------------------------------------------------------
check_dependencies() {
  log_info "Checking required dependencies..."

  local missing=()

  for cmd in kind kubectl kustomize; do
    if ! command -v "$cmd" &>/dev/null; then
      missing+=("$cmd")
    fi
  done

  if [[ ${#missing[@]} -gt 0 ]]; then
    log_error "Missing required dependencies: ${missing[*]}"
    echo "Please install the missing tools and try again."
    exit 1
  fi

  log_success "All dependencies are available"
}

# -----------------------------------------------------------------------------
# create_kind_cluster
# Create KinD cluster with ingress port mappings
# -----------------------------------------------------------------------------
create_kind_cluster() {
  log_info "Creating KinD cluster '${KIND_CLUSTER_NAME}'..."

  # Check if cluster already exists
  if kind get clusters 2>/dev/null | grep -q "^${KIND_CLUSTER_NAME}$"; then
    log_info "Cluster '${KIND_CLUSTER_NAME}' already exists, skipping creation"
    kubectl cluster-info --context "kind-${KIND_CLUSTER_NAME}" || {
      log_error "Cluster exists but is not accessible. Delete it with: kind delete cluster --name ${KIND_CLUSTER_NAME}"
      exit 1
    }
    return 0
  fi

  # Create cluster config
  local config_file
  config_file=$(mktemp)
  cat > "$config_file" <<EOF
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  image: kindest/node:${KUBERNETES_VERSION}
  extraPortMappings:
  - containerPort: 80
    hostPort: 80
    protocol: TCP
  - containerPort: 443
    hostPort: 443
    protocol: TCP
EOF

  kind create cluster --name "${KIND_CLUSTER_NAME}" --config "$config_file"
  rm -f "$config_file"

  # Set kubectl context
  kubectl cluster-info --context "kind-${KIND_CLUSTER_NAME}"

  log_success "KinD cluster '${KIND_CLUSTER_NAME}' created"
}

# -----------------------------------------------------------------------------
# install_gateway_api
# Install Gateway API CRDs (standard + experimental)
# -----------------------------------------------------------------------------
install_gateway_api() {
  log_info "Installing Gateway API CRDs..."

  # Install standard Gateway API CRDs
  log_wait "Installing standard Gateway API CRDs (${GATEWAY_API_VERSION})..."
  kubectl apply -f "https://github.com/kubernetes-sigs/gateway-api/releases/download/${GATEWAY_API_VERSION}/standard-install.yaml"

  # Wait for Gateway CRD
  wait_for_crd "gateways.gateway.networking.k8s.io" 60s

  log_success "Gateway API CRDs installed"
}

# -----------------------------------------------------------------------------
# install_cert_manager
# Install cert-manager and wait for readiness
# -----------------------------------------------------------------------------
install_cert_manager() {
  log_info "Installing cert-manager ${CERT_MANAGER_VERSION}..."

  kubectl create namespace cert-manager --dry-run=client -o yaml | kubectl apply -f -

  kubectl apply -f "https://github.com/cert-manager/cert-manager/releases/download/${CERT_MANAGER_VERSION}/cert-manager.yaml"

  log_wait "Waiting for cert-manager to be ready..."
  wait_for_pods "cert-manager" "app in (cert-manager,webhook)" 180s

  # Wait for webhook to be ready
  sleep 5

  log_success "cert-manager installed"
}

# -----------------------------------------------------------------------------
# setup_cert_manager_pki
# Create PKI chain using resources from config/overlays/odh-test/cert-manager/:
#   - opendatahub-selfsigned-issuer (ClusterIssuer)
#   - opendatahub-ca (Certificate in cert-manager ns)
#   - opendatahub-ca-issuer (ClusterIssuer)
# -----------------------------------------------------------------------------
setup_cert_manager_pki() {
  log_info "Setting up cert-manager PKI chain..."

  # Apply PKI resources from odh-test overlay
  log_wait "Applying cert-manager PKI resources..."
  kubectl apply -k "${PROJECT_ROOT}/config/overlays/odh-test/cert-manager"

  # Wait for self-signed issuer to be ready
  log_wait "Waiting for opendatahub-selfsigned-issuer to be ready..."
  kubectl wait --for=condition=Ready clusterissuer/opendatahub-selfsigned-issuer --timeout=60s

  # Wait for CA certificate to be issued
  log_wait "Waiting for opendatahub-ca certificate to be issued..."
  kubectl wait --for=condition=Ready certificate/opendatahub-ca -n cert-manager --timeout=120s

  # Wait for CA issuer to be ready
  log_wait "Waiting for opendatahub-ca-issuer to be ready..."
  kubectl wait --for=condition=Ready clusterissuer/opendatahub-ca-issuer --timeout=60s

  log_success "cert-manager PKI chain created"
}

# -----------------------------------------------------------------------------
# install_istio
# Install Istio with Gateway API support using official istioctl
# Minimal profile - only what's needed for Gateway API provider
# -----------------------------------------------------------------------------
install_istio() {
  log_info "Installing Istio ${ISTIO_VERSION} with Gateway API support..."

  # Download and install istioctl
  local istio_dir
  istio_dir=$(mktemp -d)
  pushd "$istio_dir" > /dev/null

  log_wait "Downloading istioctl ${ISTIO_VERSION}..."
  curl -sSL https://istio.io/downloadIstio | ISTIO_VERSION="${ISTIO_VERSION}" sh -

  local istioctl_bin="${istio_dir}/istio-${ISTIO_VERSION}/bin/istioctl"

  # Install Istio with minimal profile for Gateway API
  # Reduce pilot resource requests to fit in resource-constrained KinD
  log_wait "Installing Istio with minimal profile..."
  "$istioctl_bin" install -y --set profile=minimal \
    --set values.pilot.env.PILOT_ENABLE_GATEWAY_API=true \
    --set values.pilot.env.ENABLE_GATEWAY_API_INFERENCE_EXTENSION=true \
    --set values.pilot.resources.requests.cpu=100m \
    --set values.pilot.resources.requests.memory=256Mi

  popd > /dev/null
  rm -rf "$istio_dir"

  # Wait for istiod to be ready
  log_wait "Waiting for Istio to be ready..."
  wait_for_pods "istio-system" "app=istiod" 240s

  log_success "Istio installed"
}

# -----------------------------------------------------------------------------
# install_lws
# Install LeaderWorkerSet operator
# -----------------------------------------------------------------------------
install_lws() {
  log_info "Installing LeaderWorkerSet operator ${LWS_VERSION}..."

  kubectl apply --server-side -f "https://github.com/kubernetes-sigs/lws/releases/download/${LWS_VERSION}/manifests.yaml"

  # Remove CPU requests from LWS controller to fit in resource-constrained KinD
  log_wait "Removing resource requests from LWS controller..."
  kubectl patch deployment lws-controller-manager -n lws-system --type=json \
    -p='[{"op": "remove", "path": "/spec/template/spec/containers/0/resources/requests"}]' 2>/dev/null || true

  log_wait "Waiting for LWS controller to be ready..."
  kubectl wait deploy/lws-controller-manager -n lws-system --for=condition=available --timeout=300s

  log_success "LWS operator installed"
}

# -----------------------------------------------------------------------------
# build_and_load_controller
# Build KServe controller image and load into KinD
# -----------------------------------------------------------------------------
build_and_load_controller() {
  log_info "Building KServe controller image '${KSERVE_CONTROLLER_IMAGE}'..."

  # Build the controller image using the Makefile target
  # GOTAGS=distro is set automatically via Makefile.overrides.mk
  log_wait "Running make docker-build-llmisvc..."
  make -C "${PROJECT_ROOT}" docker-build-llmisvc \
    KO_DOCKER_REPO="${KO_DOCKER_REPO}" \
    LLMISVC_CONTROLLER_IMG="${LLMISVC_CONTROLLER_IMG}"

  # Load image into KinD cluster
  log_wait "Loading image into KinD cluster..."
  kind load docker-image "${KSERVE_CONTROLLER_IMAGE}" --name "${KIND_CLUSTER_NAME}"

  log_success "Controller image built and loaded into KinD"
}

# -----------------------------------------------------------------------------
# deploy_odh_xks
# Apply CRDs then odh-xks overlay
# -----------------------------------------------------------------------------
deploy_odh_xks() {
  log_info "Deploying KServe with odh-xks overlay..."

  # Create KServe namespace
  kubectl create namespace "${KSERVE_NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -

  # Apply KServe CRDs first
  log_wait "Applying KServe CRDs..."
  kustomize build "${PROJECT_ROOT}/config/crd/full/llmisvc" | kubectl apply --server-side=true --force-conflicts -f -

  # Wait for InferenceService CRD
  wait_for_crd "llminferenceservices.serving.kserve.io" 60s

  # Apply odh-xks overlay
  log_wait "Applying odh-xks overlay..."
  kustomize build "${PROJECT_ROOT}/config/overlays/odh-xks" | kubectl apply --server-side=true --force-conflicts -f -

  # Patch the deployment to use the local controller image
  log_wait "Patching deployment to use local controller image '${KSERVE_CONTROLLER_IMAGE}'..."
  kubectl set image deployment/llmisvc-controller-manager \
    manager="${KSERVE_CONTROLLER_IMAGE}" \
    -n "${KSERVE_NAMESPACE}"

  # Set imagePullPolicy to Never for local images
  kubectl patch deployment llmisvc-controller-manager \
    -n "${KSERVE_NAMESPACE}" \
    --type=json \
    -p='[{"op": "replace", "path": "/spec/template/spec/containers/0/imagePullPolicy", "value": "Never"}]'

  # Force rollout restart to pick up new image (needed for re-runs)
  log_wait "Restarting deployment to pick up new image..."
  kubectl rollout restart deployment/llmisvc-controller-manager -n "${KSERVE_NAMESPACE}"

  # Wait for rollout to complete
  log_wait "Waiting for KServe controller rollout to complete..."
  kubectl rollout status deployment/llmisvc-controller-manager -n "${KSERVE_NAMESPACE}" --timeout=300s

  log_success "KServe deployed with odh-xks overlay"
}

# -----------------------------------------------------------------------------
# create_gateway
# Create GatewayClass and Gateway resources with CA bundle mount configuration
# -----------------------------------------------------------------------------
create_gateway() {
  log_info "Creating Gateway resources..."

  # Create Istio GatewayClass
  log_wait "Creating istio GatewayClass..."
  kubectl apply -f - <<'EOF'
apiVersion: gateway.networking.k8s.io/v1
kind: GatewayClass
metadata:
  name: istio
spec:
  controllerName: istio.io/gateway-controller
EOF

  # Create KServe Gateway with parametersRef to mount CA bundle
  log_wait "Creating inference-gateway Gateway..."
  kubectl apply -f - <<EOF
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: inference-gateway
  namespace: ${KSERVE_NAMESPACE}
spec:
  gatewayClassName: istio
  listeners:
    - name: http
      port: 80
      protocol: HTTP
      allowedRoutes:
        namespaces:
          from: All
  infrastructure:
    labels:
      serving.kserve.io/gateway: kserve-ingress-gateway
    parametersRef:
      group: ""
      kind: ConfigMap
      name: inference-gateway-config
EOF

  # Wait for Gateway to be programmed
  log_wait "Waiting for Gateway to be programmed..."
  kubectl wait --for=condition=Programmed gateway/inference-gateway -n "${KSERVE_NAMESPACE}" --timeout=120s

  log_success "Gateway resources created"
}

# -----------------------------------------------------------------------------
# setup_ca_bundle
# Create CA bundle ConfigMap for workloads that need to trust the CA
# Also create the Gateway deployment configuration ConfigMap
# -----------------------------------------------------------------------------
setup_ca_bundle() {
  log_info "Setting up CA bundle ConfigMap..."

  # Extract CA certificate from the secret
  local ca_cert
  ca_cert=$(kubectl get secret opendatahub-ca -n cert-manager -o jsonpath='{.data.ca\.crt}' 2>/dev/null || \
            kubectl get secret opendatahub-ca -n cert-manager -o jsonpath='{.data.tls\.crt}')

  if [[ -z "$ca_cert" ]]; then
    log_error "Could not extract CA certificate from opendatahub-ca secret"
    return 1
  fi

  # Create CA bundle ConfigMap in KServe namespace
  # Use 'ca.crt' as the key to match the mount path /var/run/secrets/opendatahub/ca.crt
  kubectl create configmap odh-ca-bundle \
    --from-literal=ca.crt="$(echo "$ca_cert" | base64 -d)" \
    -n "${KSERVE_NAMESPACE}" \
    --dry-run=client -o yaml | kubectl apply -f -

  # Create Gateway deployment configuration ConfigMap for Istio
  # This configures the Gateway pod to mount the CA bundle
  log_wait "Creating Gateway deployment configuration..."
  kubectl apply -f - <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: inference-gateway-config
  namespace: ${KSERVE_NAMESPACE}
data:
  deployment: |
    spec:
      template:
        spec:
          volumes:
          - name: odh-ca-bundle
            configMap:
              name: odh-ca-bundle
          containers:
          - name: istio-proxy
            volumeMounts:
            - name: odh-ca-bundle
              mountPath: /var/run/secrets/opendatahub
              readOnly: true
EOF

  log_success "CA bundle ConfigMaps created"
}

# -----------------------------------------------------------------------------
# create_test_namespace
# Create the E2E test namespace for running tests
# -----------------------------------------------------------------------------
create_test_namespace() {
  log_info "Creating E2E test namespace..."

  kubectl create namespace kserve-ci-e2e-test --dry-run=client -o yaml | kubectl apply -f -

  log_success "E2E test namespace 'kserve-ci-e2e-test' created"
}

# -----------------------------------------------------------------------------
# print_verification_steps
# Print commands to verify the installation
# -----------------------------------------------------------------------------
print_verification_steps() {
  echo ""
  echo "=========================================="
  echo "Installation Complete!"
  echo "=========================================="
  echo ""
  echo "Verification commands:"
  echo ""
  echo "  # Check KServe controller is running"
  echo "  kubectl get pods -n ${KSERVE_NAMESPACE}"
  echo ""
  echo "  # Check webhook certificate is issued"
  echo "  kubectl get certificate -n ${KSERVE_NAMESPACE}"
  echo ""
  echo "  # Check Gateway is programmed"
  echo "  kubectl get gateway -n ${KSERVE_NAMESPACE}"
  echo ""
  echo "  # Check ClusterIssuers"
  echo "  kubectl get clusterissuers"
  echo ""
  echo "To delete the cluster:"
  echo "  kind delete cluster --name ${KIND_CLUSTER_NAME}"
  echo ""
}

# -----------------------------------------------------------------------------
# main
# Orchestrate installation order
# -----------------------------------------------------------------------------
main() {
  echo ""
  echo "=========================================="
  echo "KServe odh-xks Setup for KinD"
  echo "=========================================="
  echo ""
  echo "Configuration:"
  echo "  Cluster Name:        ${KIND_CLUSTER_NAME}"
  echo "  Kubernetes Version:  ${KUBERNETES_VERSION}"
  echo "  Istio Version:       ${ISTIO_VERSION}"
  echo "  cert-manager:        ${CERT_MANAGER_VERSION}"
  echo "  LWS Version:         ${LWS_VERSION}"
  echo "  Gateway API:         ${GATEWAY_API_VERSION}"
  echo "  KServe Namespace:    ${KSERVE_NAMESPACE}"
  echo ""

  # 1. Check dependencies
  check_dependencies

  # 2. Create KinD cluster
  create_kind_cluster

  # 3. Install Gateway API CRDs
  install_gateway_api

  # 4. Install cert-manager
  install_cert_manager

  # 5. Setup cert-manager PKI (requires cert-manager)
  setup_cert_manager_pki

  # 6. Install Istio (requires Gateway API CRDs)
  install_istio

  # 7. Install LWS
  install_lws

  # 8. Build and load controller image
  build_and_load_controller

  # 9. Deploy odh-xks overlay (requires cert-manager PKI)
  deploy_odh_xks

  # 10. Setup CA bundle ConfigMap (must be before Gateway as Gateway references it)
  setup_ca_bundle

  # 11. Create Gateway resources
  create_gateway

  # 12. Create E2E test namespace
  create_test_namespace

  # Print verification steps
  print_verification_steps
}

# Run main function
main "$@"
