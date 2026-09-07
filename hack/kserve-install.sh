#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd 2>/dev/null)"
source "${SCRIPT_DIR}/setup/common.sh"
REPO_ROOT="$(find_repo_root "${SCRIPT_DIR}")"
source "${REPO_ROOT}/kserve-deps.env"
source "${REPO_ROOT}/hack/setup/global-vars.env"

TYPES=()
MODE="serverless"
INSTALL_METHOD="helm"
ENABLE_KEDA=false
DEPS_ONLY=false
UNINSTALL=false
KSERVE_INSTALL_CI=false
USE_LOCAL_CHARTS=false
ENABLE_KSERVE=false
ENABLE_LOCALMODEL=false
ENABLE_LLMISVC=false
SET_KSERVE_VERSION=""
SET_KSERVE_REGISTRY=""
LLMISVC_SCALING=""

Help() {
   echo "KServe installation script"
   echo ""
   echo "Options:"
   echo "  --type TYPE[,...]      Types: kserve,localmodel,llmisvc (default: kserve)"
   echo "  --serverless, --knative, -s    Serverless mode (default, with Istio)"
   echo "  --raw, --standard, -r          Standard mode (RawDeployment)"
   echo "  --helm                         Use Helm (default)"
   echo "  --kustomize                    Use Kustomize"
   echo "  --local-chart, -lc             Use local charts (helm only)"
   echo "  --kserve-version VER           Override KServe version (default: from kserve-deps.env)"
   echo "  --kserve-registry REG          Override image registry (kustomize only, e.g., quay.io/myuser)"
   echo "  --no-runtimes                   Skip installing ClusterServingRuntimes"
   echo "  --keda, -k                     Enable KEDA (standard mode only)"
   echo "  --scaling MODE                 Autoscaling for LLMISvc: keda or hpa (requires --type llmisvc)"
   echo "  --deps-only, -d                Install dependencies only"
   echo "  --uninstall, -u                Uninstall all"
   echo "  --ci                           CI mode (skip frozen install and LocalModel version checks)"
   echo ""
}

while [[ $# -gt 0 ]]; do
  case $1 in
    -h|--help)
      Help
      exit 0
      ;;
    --type)
      IFS=',' read -ra TYPES <<< "$2"
      shift 2
      ;;
    --serverless|--knative|-s)
      MODE="serverless"
      shift
      ;;
    --raw|--standard|-r)
      MODE="raw"
      shift
      ;;
    --helm)
      INSTALL_METHOD="helm"
      shift
      ;;
    --kustomize)
      INSTALL_METHOD="kustomize"
      shift
      ;;
    --local-chart|-lc)
      USE_LOCAL_CHARTS=true
      shift
      ;;
    --kserve-version)
      SET_KSERVE_VERSION="$2"
      shift 2
      ;;
    --kserve-registry)
      SET_KSERVE_REGISTRY="$2"
      shift 2
      ;;
    --no-runtimes)
      INSTALL_RUNTIMES=false
      shift
      ;;
    --keda|-k)
      ENABLE_KEDA=true
      shift
      ;;
    --scaling)
      LLMISVC_SCALING="$2"
      shift 2
      ;;
    --deps-only|-d)
      DEPS_ONLY=true
      shift
      ;;
    --uninstall|-u)
      UNINSTALL=true
      shift
      ;;
    --ci)
      KSERVE_INSTALL_CI=true
      shift
      ;;
    *)
      log_error "Unknown option: $1"
      Help
      exit 1
      ;;
  esac
done

if ! is_positive "$ENABLE_KEDA" && [[ ${#TYPES[@]} -eq 0 ]]; then
  [[ ${#TYPES[@]} -eq 0 ]] && TYPES=("kserve")
fi

# Validate types and auto-enable corresponding configs
for type in "${TYPES[@]}"; do
  if [[ "$type" != "kserve" && "$type" != "localmodel" && "$type" != "llmisvc" ]]; then
    log_error "Invalid type: $type. Must be one of: kserve, localmodel, llmisvc"
    exit 1
  fi

  # Auto-enable configs for each type
  [[ "$type" == "kserve" ]] && ENABLE_KSERVE=true
  [[ "$type" == "localmodel" ]] && ENABLE_LOCALMODEL=true
  [[ "$type" == "llmisvc" ]] && ENABLE_LLMISVC=true
done

# Set runtime configs after all types are processed
INSTALL_RUNTIMES="${INSTALL_RUNTIMES:-$ENABLE_KSERVE}"
INSTALL_LLMISVC_CONFIGS="${INSTALL_LLMISVC_CONFIGS:-$ENABLE_LLMISVC}"

# Export all configuration variables
export ENABLE_KSERVE
export ENABLE_LOCALMODEL
export ENABLE_LLMISVC
export INSTALL_RUNTIMES
export INSTALL_LLMISVC_CONFIGS
export SET_KSERVE_VERSION
export SET_KSERVE_REGISTRY
export USE_LOCAL_CHARTS
export KSERVE_INSTALL_CI

# Normalize mode: serverless/knative → Knative, raw/standard → Standard
case "$MODE" in
  serverless|knative) NORMALIZED_MODE="Knative"; USER_MODE="serverless" ;;
  raw|standard) NORMALIZED_MODE="Standard"; USER_MODE="raw" ;;
  *) log_error "Invalid mode: $MODE"; exit 1 ;;
esac

export DEPLOYMENT_MODE="${NORMALIZED_MODE}"

# Validate --scaling flag
if [[ -n "$LLMISVC_SCALING" ]]; then
  if [[ "$LLMISVC_SCALING" != "keda" && "$LLMISVC_SCALING" != "hpa" ]]; then
    log_error "Invalid scaling mode: $LLMISVC_SCALING. Must be 'keda' or 'hpa'"
    exit 1
  fi
  if ! is_positive "$ENABLE_LLMISVC"; then
    log_error "--scaling requires --type llmisvc"
    exit 1
  fi
fi

# Validate dependencies
is_positive "$USE_LOCAL_CHARTS" && [[ $INSTALL_METHOD != "helm" ]] && { log_error "Local chart requires helm mode"; exit 1; }
[[ -n "${SET_KSERVE_REGISTRY}" && $INSTALL_METHOD != "kustomize" ]] && { log_error "--kserve-registry requires kustomize mode"; exit 1; }

INSTALL_SCRIPT_DIR="${REPO_ROOT}/hack/setup/quick-install"
USE_FROZEN_INSTALL=false
# Use install/${VERSION}/ frozen scripts when installing a different release without a custom registry.
# Requires --kserve-version, version != kserve-deps.env, no --kserve-registry, and install/${VERSION}/ on disk.
# Skipped in --ci mode (e.g. image tag is a git SHA, not a release bundle).
if ! is_positive "$KSERVE_INSTALL_CI" \
  && [[ -n "${SET_KSERVE_VERSION}" && "${SET_KSERVE_VERSION}" != "${KSERVE_VERSION}" && -z "${SET_KSERVE_REGISTRY}" ]]; then
  if [[ ! -d "${REPO_ROOT}/install/${SET_KSERVE_VERSION}" ]]; then
    if [[ $INSTALL_METHOD == "helm" ]]; then
      log_error "KServe Helm chart for ${SET_KSERVE_VERSION} is not found. Checkout a release version"
    else
      log_error "KServe Manifests for ${SET_KSERVE_VERSION} is not found. Checkout a release version"
    fi
    exit 1
  fi
  is_positive "$USE_LOCAL_CHARTS" && { log_error "Local charts are not supported with install/${SET_KSERVE_VERSION}/ scripts"; exit 1; }
  INSTALL_SCRIPT_DIR="${REPO_ROOT}/install/${SET_KSERVE_VERSION}"
  USE_FROZEN_INSTALL=true
  log_info "Using frozen install scripts from install/${SET_KSERVE_VERSION}/"
fi

show_installation_plan() {
  echo ""
  echo "========================================"
  echo "  Installation Plan"
  echo "========================================"
  echo ""
  echo "📋 Method: ${INSTALL_METHOD}"
  is_positive "$USE_LOCAL_CHARTS" && echo "📦 Chart: Local (${REPO_ROOT}/charts/)"
  echo "📦 Types: ${TYPES[*]}"
  echo ""
  echo "Common Dependencies:"
  echo "  - Cert-Manager"
  echo ""

  echo "Will install:"
  for type in "${TYPES[@]}"; do
    case $type in
      kserve)
        echo "  • KServe (${USER_MODE} mode)"
        if [[ $USER_MODE == "serverless" ]]; then
          echo "    - Dependencies: Istio, Knative"
        fi
        is_positive "$ENABLE_KEDA" && echo "    - With KEDA autoscaling"
        ! is_positive "$DEPS_ONLY" && is_positive "$INSTALL_RUNTIMES" && echo "    - With ClusterServingRuntimes"
        ;;
      localmodel)
        echo "  • LocalModel (default settings)"
        ;;
      llmisvc)
        echo "  • LLMIsvc"
        echo "    - Dependencies: Gateway API, LWS Operator, Envoy Gateway"
        if [[ "${LLMISVC_SCALING}" == "keda" ]]; then
          echo "    - Autoscaling: KEDA (Prometheus + KEDA + WVA)"
        elif [[ "${LLMISVC_SCALING}" == "hpa" ]]; then
          echo "    - Autoscaling: HPA (Prometheus + Prometheus Adapter + WVA)"
        fi
        ! is_positive "$DEPS_ONLY" && is_positive "$INSTALL_LLMISVC_CONFIGS" && echo "    - With LLMIsvc Configs"
        ;;
    esac
  done
  echo ""
  echo "========================================"
  echo ""
}

uninstall_all() {
  log_info "Uninstalling all components..."

  local scripts=(
    "hack/setup/infra/manage.kserve-helm.sh"
    "hack/setup/infra/manage.kserve-kustomize.sh"
    "hack/setup/infra/manage.wva-kustomize.sh"
    "hack/setup/infra/manage.prometheus-adapter-helm.sh"
    "hack/setup/infra/manage.prometheus-helm.sh"
    "hack/setup/infra/manage.keda-otel-addon-helm.sh"
    "hack/setup/infra/manage.opentelemetry-helm.sh"
    "hack/setup/infra/manage.keda-helm.sh"
    "hack/setup/infra/knative/manage.knative-operator-helm.sh"
    "hack/setup/infra/manage.istio-helm.sh"
    "hack/setup/infra/manage.envoy-ai-gateway-helm.sh"
    "hack/setup/infra/manage.envoy-gateway-helm.sh"
    "hack/setup/infra/gateway-api/manage.gateway-api-crd.sh"
    "hack/setup/infra/manage.cert-manager-helm.sh"
    "hack/setup/infra/manage.lws-operator.sh"
  )

  for script in "${scripts[@]}"; do
    ${REPO_ROOT}/${script} --uninstall 2>/dev/null || true
  done

  log_success "All components uninstalled"
}

LOCALMODEL_MIN_VERSION="v0.21.0"
if is_positive "$ENABLE_LOCALMODEL" && [[ -n "${SET_KSERVE_VERSION}" ]] && ! is_positive "$KSERVE_INSTALL_CI"; then
  requested_version="${SET_KSERVE_VERSION:-${KSERVE_VERSION}}"
  if ! version_gte "${requested_version}" "${LOCALMODEL_MIN_VERSION}"; then
    log_error "LocalModel install requires KServe version >= ${LOCALMODEL_MIN_VERSION} (requested: ${requested_version})"
    exit 1
  fi
fi

if is_positive "$UNINSTALL"; then
  uninstall_all
  exit 0
fi

show_installation_plan

install_dependencies() {
  log_info "Installing dependencies..."

  # Individual installation
  for type in "${TYPES[@]}"; do
    case $type in
      kserve)
        if [[ $USER_MODE == "serverless" ]]; then
          ${INSTALL_SCRIPT_DIR}/kserve-knative-mode-dependency-install.sh
        else
          ${INSTALL_SCRIPT_DIR}/kserve-standard-mode-dependency-install.sh
        fi

        ;;
      llmisvc)
        ${INSTALL_SCRIPT_DIR}/llmisvc-dependency-install.sh
        ;;
    esac
  done

  # Install KEDA dependencies if enabled
  if is_positive "${ENABLE_KEDA}"; then
    ${INSTALL_SCRIPT_DIR}/keda-dependency-install.sh
  fi

  # Install LLMISvc autoscaling dependencies if enabled
  if [[ -n "${LLMISVC_SCALING}" ]]; then
    if [[ "${LLMISVC_SCALING}" == "keda" ]]; then
      ${INSTALL_SCRIPT_DIR}/llmisvc-autoscaling-keda-dependency-install.sh
    else
      ${INSTALL_SCRIPT_DIR}/llmisvc-autoscaling-hpa-dependency-install.sh
    fi
  fi

  log_success "Dependencies installed"
}

install_dependencies

if is_positive "$DEPS_ONLY"; then
  echo ""
  echo "✅ Dependencies installation complete!"
  exit 0
fi

# Install all enabled types together (single execution)
if [[ ${#TYPES[@]} -gt 0 ]]; then
  log_info "Installing: ${TYPES[*]}..."
  if is_positive "$USE_FROZEN_INSTALL"; then
    # Frozen scripts embed kserve (or llmisvc) manifests only — localmodel needs kustomize.
    if is_positive "$ENABLE_KSERVE"; then
      if [[ $USER_MODE == "serverless" ]]; then
        ENABLE_LOCALMODEL=false ${INSTALL_SCRIPT_DIR}/kserve-knative-mode-full-install-with-manifests.sh
      else
        ENABLE_LOCALMODEL=false ${INSTALL_SCRIPT_DIR}/kserve-standard-mode-full-install-with-manifests.sh
      fi
    fi
    if is_positive "$ENABLE_LOCALMODEL"; then
      log_info "Installing LocalModel via frozen manifests..."
      ${INSTALL_SCRIPT_DIR}/localmodel-full-install-with-manifests.sh
    fi
    if is_positive "$ENABLE_LLMISVC"; then
      ${INSTALL_SCRIPT_DIR}/llmisvc-full-install-with-manifests.sh
    fi
  elif [[ $INSTALL_METHOD == "helm" ]]; then
    ${REPO_ROOT}/hack/setup/infra/manage.kserve-helm.sh
  else
    ${REPO_ROOT}/hack/setup/infra/manage.kserve-kustomize.sh
  fi

  # Configure autoscaling settings in inferenceservice-config ConfigMap
  if [[ -n "${LLMISVC_SCALING}" ]]; then
    AUTOSCALING_PROM_URL="${WVA_PROMETHEUS_URL:-https://prometheus-kube-prometheus-prometheus.${PROMETHEUS_NAMESPACE:-monitoring}:9090}"
    AUTOSCALING_PROM_UNSAFE_SSL="${WVA_PROMETHEUS_UNSAFE_SSL:-true}"
    log_info "Configuring autoscaling-wva-controller-config with Prometheus URL: ${AUTOSCALING_PROM_URL}"
    update_isvc_config \
      "autoscaling-wva-controller-config.prometheus.url=${AUTOSCALING_PROM_URL}" \
      "autoscaling-wva-controller-config.prometheus.unsafeSsl=${AUTOSCALING_PROM_UNSAFE_SSL}"
  fi

  log_success "Installation complete: ${TYPES[*]}"
  echo ""
  echo "========================================"
  echo "  ✅ Installation Complete!"
  echo "========================================"
  echo ""
  echo "📝 Verify installation:"
  echo "   kubectl get pods -n kserve"
  echo ""
  echo "📚 Documentation:"
  echo "   https://kserve.github.io/website/"
  echo ""
  echo "========================================"
fi
