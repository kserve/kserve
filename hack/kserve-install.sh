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
USE_LOCAL_CHARTS=false
ENABLE_KSERVE=false
ENABLE_LOCALMODEL=false
ENABLE_LLMISVC=false
SET_KSERVE_VERSION=""
SET_KSERVE_REGISTRY=""

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
   echo "  --deps-only, -d                Install dependencies only"
   echo "  --uninstall, -u                Uninstall all"
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
    --deps-only|-d)
      DEPS_ONLY=true
      shift
      ;;
    --uninstall|-u)
      UNINSTALL=true
      shift
      ;;
    *)
      log_error "Unknown option: $1"
      Help
      exit 1
      ;;
  esac
done

[[ ${#TYPES[@]} -eq 0 ]] && TYPES=("kserve")

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

# Normalize mode: serverless/knative → Knative, raw/standard → Standard
case "$MODE" in
  serverless|knative) NORMALIZED_MODE="Knative"; USER_MODE="serverless" ;;
  raw|standard) NORMALIZED_MODE="Standard"; USER_MODE="raw" ;;
  *) log_error "Invalid mode: $MODE"; exit 1 ;;
esac

export DEPLOYMENT_MODE="${NORMALIZED_MODE}"

# Validate dependencies
[[ $USE_LOCAL_CHARTS == true && $INSTALL_METHOD != "helm" ]] && { log_error "Local chart requires helm mode"; exit 1; }
[[ -n "${SET_KSERVE_REGISTRY}" && $INSTALL_METHOD != "kustomize" ]] && { log_error "--kserve-registry requires kustomize mode"; exit 1; }

show_installation_plan() {
  echo ""
  echo "========================================"
  echo "  Installation Plan"
  echo "========================================"
  echo ""
  echo "📋 Method: ${INSTALL_METHOD}"
  [[ $USE_LOCAL_CHARTS == true ]] && echo "📦 Chart: Local (${REPO_ROOT}/charts/)"
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
        [[ $ENABLE_KEDA == true ]] && echo "    - With KEDA autoscaling"
        [[ $INSTALL_RUNTIMES == "true" ]] && echo "    - With ClusterServingRuntimes"
        ;;
      localmodel)
        echo "  • LocalModel (default settings)"
        ;;
      llmisvc)
        echo "  • LLMIsvc"
        echo "    - Dependencies: Gateway API, LWS Operator, Envoy Gateway"
        [[ $INSTALL_LLMISVC_CONFIGS == "true" ]] && echo "    - With LLMIsvc Configs"
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

if [[ $UNINSTALL == true ]]; then
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
          ${REPO_ROOT}/hack/setup/quick-install/kserve-knative-mode-dependency-install.sh
        else
          ${REPO_ROOT}/hack/setup/quick-install/kserve-standard-mode-dependency-install.sh
        fi

        ;;
      llmisvc)
        ${REPO_ROOT}/hack/setup/quick-install/llmisvc-dependency-install.sh
        ;;
    esac
  done

  # Install KEDA dependencies if enabled
  if is_positive "${ENABLE_KEDA}"; then
    ${REPO_ROOT}/hack/setup/quick-install/keda-dependency-install.sh
  fi

  
  log_success "Dependencies installed"
}

install_dependencies

if [[ $DEPS_ONLY == true ]]; then
  echo ""
  echo "✅ Dependencies installation complete!"
  exit 0
fi

# Install all enabled types together (single execution)
log_info "Installing: ${TYPES[*]}..."
if [[ $INSTALL_METHOD == "helm" ]]; then  
  ${REPO_ROOT}/hack/setup/infra/manage.kserve-helm.sh
else
  ${REPO_ROOT}/hack/setup/infra/manage.kserve-kustomize.sh
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
