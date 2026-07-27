#!/usr/bin/env bash
set -euo pipefail

# ---------------------------------------------------------------------------
# Defaults
# ---------------------------------------------------------------------------
PLATFORM="${PLATFORM:-xks}"
KSERVE_NAMESPACE="${KSERVE_NAMESPACE:-opendatahub}"
KSERVE_MODULE_IMG="${KSERVE_MODULE_IMG:-}"

# Prefer oc on OpenShift, fall back to kubectl
if command -v oc &>/dev/null; then
  KUBECTL="oc"
else
  KUBECTL="kubectl"
fi

is_openshift() {
  ${KUBECTL} get crd routes.route.openshift.io &>/dev/null
}

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd)"
KSERVE_INFRA_DIR="${PROJECT_ROOT}/hack/setup/infra"

# Source common utilities (also loads kserve-deps.env and global-vars.env)
source "${PROJECT_ROOT}/hack/setup/common.sh"

# Source operator subscription configurations
source "${PROJECT_ROOT}/test/scripts/openshift-ci/operator-subscriptions.sh"

# ---------------------------------------------------------------------------
# Platform-specific dependency definitions
# Subscription names must match kserve-module/pkg/kservemodule/dependencies.go
# ---------------------------------------------------------------------------

# OLM catalog configuration for OCP operator installation
readonly REDHAT_OPERATOR_CATALOG="redhat-operators"
readonly OPERATOR_CATALOG_NAMESPACE="openshift-marketplace"

# OLM Subscription definitions (OCP only)
# Format: "name|namespace|channel|install_mode"
readonly SUB_CERT_MANAGER="${CERT_MANAGER_NAME}|${CERT_MANAGER_NAMESPACE}|${CERT_MANAGER_CHANNEL}|AllNamespaces"
readonly SUB_LWS="${LWS_NAME}|${LWS_NAMESPACE}|${LWS_CHANNEL}|OwnNamespace"
readonly SUB_RHCL="${RHCL_NAME}|${RHCL_NAMESPACE}|${RHCL_CHANNEL}|AllNamespaces"
readonly SUB_CMA="${CMA_NAME}|${CMA_NAMESPACE}|${CMA_CHANNEL}|AllNamespaces"

# --- Per-platform component lists ---
# xks: install via helm scripts from hack/setup/infra
XKS_HELM_SCRIPTS=(
  "${KSERVE_INFRA_DIR}/gateway-api/manage.gateway-api-crd.sh"
  "${KSERVE_INFRA_DIR}/manage.cert-manager-helm.sh"
  "${KSERVE_INFRA_DIR}/manage.istio-helm.sh"
  "${KSERVE_INFRA_DIR}/manage.lws-operator.sh"
)

# ocp: install via OLM subscription (fast, no CSV wait)
OCP_SUBSCRIPTIONS=(
  "${SUB_CERT_MANAGER}"
  "${SUB_LWS}"
  "${SUB_RHCL}"
  "${SUB_CMA}"
)

# ---------------------------------------------------------------------------
# Parse arguments
# ---------------------------------------------------------------------------
CLEANUP=false
SKIP_DEPS=false

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --platform)   PLATFORM="$2"; shift 2 ;;
      --image)
        if [[ ! "$2" =~ ^[a-zA-Z0-9._/:@-]+$ ]]; then
          log_error "Invalid image format: $2"
          exit 1
        fi
        KSERVE_MODULE_IMG="$2"; shift 2 ;;
      --cleanup)    CLEANUP=true; shift ;;
      --skip-deps)  SKIP_DEPS=true; shift ;;
      -h|--help)    usage; exit 0 ;;
      *)            echo "Unknown option: $1"; usage; exit 1 ;;
    esac
  done

  case "$PLATFORM" in
    xks|ocp) ;;
    *) log_error "Invalid platform: $PLATFORM (must be xks or ocp)"; exit 1 ;;
  esac
}

usage() {
  cat <<EOF
Usage: $(basename "$0") [OPTIONS]

Installs kserve-module dependencies and deploys the controller on an existing cluster.

Options:
  --platform xks|ocp           Target platform (default: xks)
  --image IMAGE                 Controller image (e.g. quay.io/org/kserve-module:tag)
  --cleanup                     Uninstall kserve-module and all platform dependencies
  --skip-deps                   Skip dependency installation (use when deps are already installed)
  -h, --help                    Show this help

Examples:
  # Install on XKS cluster
  $(basename "$0") --platform xks

  # Install on OCP with custom image
  $(basename "$0") --platform ocp --image quay.io/my-org/kserve-module:latest

  # Uninstall everything
  $(basename "$0") --cleanup --platform xks
EOF
}

# ---------------------------------------------------------------------------
# setup_cert_manager_pki (this is for xks only)
# ---------------------------------------------------------------------------
setup_cert_manager_pki() {
  log_info "Setting up cert-manager PKI for kserve-module..."
  ${KUBECTL} apply -k "${PROJECT_ROOT}/config/overlays/odh-test/cert-manager"
  ${KUBECTL} wait --for=condition=Ready clusterissuer/opendatahub-selfsigned-issuer --timeout=60s
  ${KUBECTL} wait --for=condition=Ready certificate/opendatahub-ca -n cert-manager --timeout=120s
  ${KUBECTL} wait --for=condition=Ready clusterissuer/opendatahub-ca-issuer --timeout=60s
  log_success "PKI chain created"
}

# ---------------------------------------------------------------------------
# install_xks_deps — install dependencies via helm scripts (xks)
# ---------------------------------------------------------------------------
install_xks_deps() {
  log_info "Installing xks dependencies via helm scripts..."
  for script in "${XKS_HELM_SCRIPTS[@]}"; do
    if [[ -f "${script}" ]]; then
      log_info "Running $(basename "${script}")..."
      bash "${script}"
    else
      log_error "Script not found: ${script}"
      return 1
    fi
  done
  # XKS needs custom PKI since there's no OpenShift CA
  setup_cert_manager_pki
}


# ---------------------------------------------------------------------------
# install_ocp_subscription — create a single OLM subscription if not present
# ---------------------------------------------------------------------------
install_ocp_subscription() {
  local sub_name="$1"
  local ns="$2"
  local channel="$3"
  local mode="${4:-AllNamespaces}"

  if ${KUBECTL} get subscription "${sub_name}" -n "${ns}" &>/dev/null; then
    log_info "Subscription ${sub_name} already exists, skipping"
    return
  fi

  log_info "Creating subscription: ${sub_name} (ns=${ns}, channel=${channel}, mode=${mode})"
  create_or_skip_namespace "${ns}"

  local target_ns_spec='targetNamespaces: []'
  if [[ "${mode}" == "OwnNamespace" ]]; then
    target_ns_spec="targetNamespaces:
    - ${ns}"
  fi

  ${KUBECTL} apply -f - <<EOF
apiVersion: operators.coreos.com/v1
kind: OperatorGroup
metadata:
  name: ${sub_name}
  namespace: ${ns}
spec:
  ${target_ns_spec}
---
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: ${sub_name}
  namespace: ${ns}
spec:
  channel: ${channel}
  name: ${sub_name}
  source: ${REDHAT_OPERATOR_CATALOG}
  sourceNamespace: ${OPERATOR_CATALOG_NAMESPACE}
  installPlanApproval: Automatic
EOF

  log_success "${sub_name} subscription created (CSV will install in background)"
}

# ---------------------------------------------------------------------------
# install_ocp_deps — install dependencies via OLM subscriptions (ocp)
# ---------------------------------------------------------------------------
install_ocp_deps() {
  log_info "Installing OCP dependencies via OLM subscriptions..."
  for entry in "${OCP_SUBSCRIPTIONS[@]}"; do
    IFS='|' read -r name ns channel mode <<< "${entry}"
    install_ocp_subscription "${name}" "${ns}" "${channel}" "${mode}"
  done
  # OCP uses OpenShift's built-in CA, no custom PKI needed
}

# ---------------------------------------------------------------------------
# cleanup_kserve_module — remove kserve CR, operands, and operator
# ---------------------------------------------------------------------------
cleanup_kserve_module() {
  log_info "Cleaning up kserve-module..."
  ${KUBECTL} delete kserve --all --ignore-not-found -n "${KSERVE_NAMESPACE}" 2>/dev/null || true

  log_info "Waiting for operand cleanup..."
  sleep 10

  local config_dir="${PROJECT_ROOT}/kserve-module/config"
  ${KUBECTL} kustomize "$config_dir" | \
    sed "s|namespace: kserve|namespace: ${KSERVE_NAMESPACE}|g" | \
    ${KUBECTL} delete --ignore-not-found -f - 2>/dev/null || true

  log_success "kserve-module cleaned up"
}

# ---------------------------------------------------------------------------
# cleanup_ocp_subscription — fully remove an OLM operator and its operands
# ---------------------------------------------------------------------------
cleanup_ocp_subscription() {
  local sub_name="$1"
  local ns="$2"

  log_info "Cleaning up ${sub_name}..."

  # Step 1: Delete operand CRs first (operator is still running to handle finalizers)
  case "${sub_name}" in
    openshift-cert-manager-operator)
      log_info "Deleting cert-manager operand..."
      ${KUBECTL} delete certmanagers.operator.openshift.io cluster --timeout=120s --ignore-not-found 2>/dev/null || true

      # Wait for cert-manager pods to be cleaned up
      log_info "Waiting for cert-manager pods cleanup..."
      ${KUBECTL} wait --for=delete pods --all -n cert-manager --timeout=120s 2>/dev/null || true

      # Clean up cert-manager resources created by this script
      ${KUBECTL} delete -k "${PROJECT_ROOT}/config/overlays/odh-test/cert-manager" --ignore-not-found 2>/dev/null || true
      ;;
    rhcl-operator)
      log_info "Deleting RHCL operands..."
      ${KUBECTL} delete kuadrants.kuadrant.io --all -n "${ns}" --timeout=60s --ignore-not-found 2>/dev/null || true
      ${KUBECTL} delete authorinos.operator.authorino.kuadrant.io --all -n "${ns}" --timeout=60s --ignore-not-found 2>/dev/null || true

      # Remove finalizers if operator is already gone and can't process them
      for kind in kuadrants.kuadrant.io authorinos.operator.authorino.kuadrant.io; do
        for cr in $(${KUBECTL} get "${kind}" -n "${ns}" -o name 2>/dev/null); do
          ${KUBECTL} patch "${cr}" -n "${ns}" --type=json -p='[{"op":"remove","path":"/metadata/finalizers"}]' 2>/dev/null || true
        done
      done
      ;;
    leader-worker-set)
      ${KUBECTL} delete leaderworkersets.operator.openshift.io --all --timeout=60s --ignore-not-found 2>/dev/null || true
      ;;
    openshift-custom-metrics-autoscaler-operator)
      ${KUBECTL} delete kedacontrollers.keda.sh --all -n "${ns}" --timeout=60s --ignore-not-found 2>/dev/null || true
      ;;
  esac

  # Step 2: Remove subscription + CSV (operator stops)
  log_info "Removing subscriptions and CSVs in ${ns}..."
  ${KUBECTL} delete subscription --all -n "${ns}" --ignore-not-found 2>/dev/null || true
  ${KUBECTL} delete csv --all -n "${ns}" --ignore-not-found 2>/dev/null || true
  ${KUBECTL} delete operatorgroup "${sub_name}" -n "${ns}" --ignore-not-found 2>/dev/null || true

  # Step 3: Clean up namespaces
  case "${sub_name}" in
    openshift-cert-manager-operator)
      log_info "Deleting cert-manager namespace..."
      ${KUBECTL} delete ns cert-manager --ignore-not-found --timeout=120s 2>/dev/null || true
      ;;
  esac

  log_info "Deleting operator namespace ${ns}..."
  ${KUBECTL} delete ns "${ns}" --ignore-not-found --timeout=120s 2>/dev/null || true

  log_success "Cleaned up ${sub_name}"
}

# ---------------------------------------------------------------------------
# cleanup_ocp_deps — remove all OCP subscriptions and their operands
# ---------------------------------------------------------------------------
cleanup_ocp_deps() {
  log_info "Cleaning up OCP dependencies..."
  for entry in "${OCP_SUBSCRIPTIONS[@]}"; do
    IFS='|' read -r name ns channel mode <<< "${entry}"
    cleanup_ocp_subscription "${name}" "${ns}"
  done
  log_success "OCP dependencies cleaned up"
}

# ---------------------------------------------------------------------------
# cleanup_xks_deps — uninstall helm-managed dependencies (reverse order)
# ---------------------------------------------------------------------------
cleanup_xks_deps() {
  log_info "Cleaning up xks dependencies..."

  # Delete PKI first (before cert-manager)
  ${KUBECTL} delete -k "${PROJECT_ROOT}/config/overlays/odh-test/cert-manager" --ignore-not-found 2>/dev/null || true

  # Uninstall in reverse order of installation
  local -a reverse_scripts=(
    "${KSERVE_INFRA_DIR}/manage.lws-operator.sh"
    "${KSERVE_INFRA_DIR}/manage.istio-helm.sh"
    "${KSERVE_INFRA_DIR}/manage.cert-manager-helm.sh"
    "${KSERVE_INFRA_DIR}/gateway-api/manage.gateway-api-crd.sh"
  )

  for script in "${reverse_scripts[@]}"; do
    if [[ -f "${script}" ]]; then
      log_info "Uninstalling $(basename "${script}")..."
      bash "${script}" --uninstall 2>/dev/null || true
    else
      log_info "Script not found, skipping: ${script}"
    fi
  done

  log_success "xks dependencies cleaned up"
}

# ---------------------------------------------------------------------------
# deploy_kserve_module
# ---------------------------------------------------------------------------
deploy_kserve_module() {
  log_info "Deploying kserve-module..."
  create_or_skip_namespace "${KSERVE_NAMESPACE}"

  local config_dir="${PROJECT_ROOT}/kserve-module/config"

  local output
  output=$(${KUBECTL} kustomize "$config_dir" | sed "s|namespace: kserve|namespace: ${KSERVE_NAMESPACE}|g")

  if [[ -n "${KSERVE_MODULE_IMG}" ]]; then
    log_info "Using custom image: ${KSERVE_MODULE_IMG}"
    output=$(echo "$output" | sed "s|image: .*odh-kserve-module-operator.*|image: ${KSERVE_MODULE_IMG}|g")
  fi

  echo "$output" | ${KUBECTL} apply --server-side=true --force-conflicts -f -

  log_info "Waiting for controller rollout..."
  ${KUBECTL} rollout status deployment/kserve-module-controller-manager \
    -n "${KSERVE_NAMESPACE}" --timeout=300s
  log_success "kserve-module deployed"
}

# ---------------------------------------------------------------------------
# main
# ---------------------------------------------------------------------------
main() {
  if is_openshift; then
    PLATFORM="ocp"
  fi

  parse_args "$@"

  echo ""
  echo "=========================================="
  echo "kserve-module E2E Setup"
  echo "=========================================="
  echo "  Platform:  ${PLATFORM}"
  echo "  Namespace: ${KSERVE_NAMESPACE}"
  [[ -n "${KSERVE_MODULE_IMG}" ]] && echo "  Image:     ${KSERVE_MODULE_IMG}"
  echo "=========================================="
  echo ""

  check_cli_exist "${KUBECTL}"


  if [[ "${CLEANUP}" == "true" ]]; then
    echo "  Action:    cleanup"
    cleanup_kserve_module
    case "$PLATFORM" in
      xks) cleanup_xks_deps ;;
      ocp) cleanup_ocp_deps ;;
    esac
    echo ""
    log_success "Cleanup complete!"
    return
  fi

  if [[ "${SKIP_DEPS}" != "true" ]]; then
    case "$PLATFORM" in
      xks)
        install_xks_deps
        ;;
      ocp)
        install_ocp_deps
        ;;
    esac
  else
    log_info "Skipping dependency installation (--skip-deps)"
  fi

  deploy_kserve_module

  echo ""
  log_success "Setup complete!"
  echo ""
  echo "  ${KUBECTL} get pods -n ${KSERVE_NAMESPACE}"
  echo "  ${KUBECTL} get crd kserves.components.platform.opendatahub.io"
  echo ""
}

main "$@"
