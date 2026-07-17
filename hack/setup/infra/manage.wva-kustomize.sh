#!/bin/bash

# Copyright 2026 The KServe Authors.
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

# Install WVA (Workload Variant Autoscaler) using Kustomize
# Usage: manage.wva-kustomize.sh [--reinstall|--uninstall]
#   or:  REINSTALL=true manage.wva-kustomize.sh
#   or:  UNINSTALL=true manage.wva-kustomize.sh
#
# Environment variables:
#   WVA_PROMETHEUS_URL - Prometheus URL for WVA
#     (default: https://prometheus-kube-prometheus-prometheus.monitoring:9090)
#
# Installs the WVA controller via kustomize from the upstream llm-d repository.
# WVA discovers workloads via annotations on HPA/ScaledObject and emits
# wva_desired_replicas metrics consumed by HPA or KEDA for scaling.

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

check_cli_exist kubectl

# VARIABLES
WVA_NAMESPACE="${WVA_NAMESPACE:-wva-system}"
WVA_PROMETHEUS_URL="${WVA_PROMETHEUS_URL:-https://prometheus-kube-prometheus-prometheus.monitoring:9090}"
WVA_REPO_URL="${WVA_REPO_URL:-https://github.com/llm-d/llm-d-workload-variant-autoscaler.git}"
# VARIABLES END

uninstall() {
    log_info "Uninstalling WVA..."

    kubectl delete deployment -l control-plane=controller-manager -n "${WVA_NAMESPACE}" 2>/dev/null || true
    kubectl delete all --all -n "${WVA_NAMESPACE}" --force --grace-period=0 2>/dev/null || true
    kubectl delete clusterrole -l app.kubernetes.io/name=workload-variant-autoscaler 2>/dev/null || true
    kubectl delete clusterrolebinding -l app.kubernetes.io/name=workload-variant-autoscaler 2>/dev/null || true
    kubectl delete namespace "${WVA_NAMESPACE}" --wait=true --timeout=60s --force --grace-period=0 2>/dev/null || true
    kubectl delete crd variantautoscalings.llmd.ai 2>/dev/null || true

    log_success "WVA uninstalled"
}

install() {
    if kubectl get deployment -n "${WVA_NAMESPACE}" -l control-plane=controller-manager 2>/dev/null | grep -q "controller-manager"; then
        if [ "$REINSTALL" = false ]; then
            log_info "WVA is already installed. Use --reinstall to reinstall."
            return 0
        else
            log_info "Reinstalling WVA..."
            uninstall
        fi
    fi

    local wva_version="${WVA_VERSION}"

    log_info "Installing WVA ${wva_version} via Kustomize..."

    # WVA's controller requires the VariantAutoscaling CRD registered for its
    # internal informers, even though KServe no longer creates VA instances.
    log_info "Installing WVA CRDs..."
    kubectl apply --server-side --force-conflicts \
        -k "${WVA_REPO_URL}/config/base/crd?ref=${wva_version}"

    local tmp_overlay
    tmp_overlay=$(mktemp -d)
    # Trap ensures cleanup on exit or error
    trap 'rm -rf "$tmp_overlay"' RETURN

    # Build a kustomization overlay that references the upstream WVA config
    # at the pinned version tag and patches in our Prometheus URL.
    cat > "$tmp_overlay/kustomization.yaml" <<EOF
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
- ${WVA_REPO_URL}/config/overlays/cluster-scoped/kubernetes?ref=${wva_version}

images:
- name: ghcr.io/llm-d/llm-d-workload-variant-autoscaler
  newTag: "${wva_version}"

patches:
# Patch the WVA config to point at our Prometheus instance
- target:
    kind: ConfigMap
    name: wva-manager-config
  patch: |-
    - op: replace
      path: /data/config.yaml
      value: |
        PROMETHEUS_BASE_URL: "${WVA_PROMETHEUS_URL}"
        PROMETHEUS_TLS_INSECURE_SKIP_VERIFY: "true"
        GLOBAL_OPT_INTERVAL: "15s"
        WVA_SCALE_TO_ZERO: "false"
# Disable metrics TLS (Prometheus scrapes over plain HTTP in the CI setup)
- target:
    kind: Deployment
    name: wva-controller-manager
  patch: |-
    - op: replace
      path: /spec/template/spec/containers/0/args
      value:
        - --leader-elect=true
        - --health-probe-bind-address=:8081
        - --config-file=/etc/wva/config.yaml
        - --metrics-bind-address=:8443
        - --metrics-secure=false
# Match ServiceMonitor to use HTTP (since metrics-secure=false)
- target:
    kind: ServiceMonitor
    name: wva-controller-manager-metrics-monitor
  patch: |-
    - op: replace
      path: /spec/endpoints
      value:
        - interval: 10s
          path: /metrics
          port: https
          scheme: http
EOF

    kubectl apply --server-side --force-conflicts -k "$tmp_overlay"

    log_success "Successfully installed WVA ${wva_version} via Kustomize"

    wait_for_pods "${WVA_NAMESPACE}" "control-plane=controller-manager" "300s"

    log_success "WVA is ready!"
}

if [ "$UNINSTALL" = true ]; then
    uninstall
    exit 0
fi

install
