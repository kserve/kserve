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

# Install WVA (Workload Variant Autoscaler) using Helm
# Usage: manage.wva-helm.sh [--reinstall|--uninstall]
#   or:  REINSTALL=true manage.wva-helm.sh
#   or:  UNINSTALL=true manage.wva-helm.sh
#
# Environment variables for custom Helm values:
#   WVA_EXTRA_ARGS - Additional helm install arguments
#   WVA_PROMETHEUS_URL - Prometheus URL for WVA (default: http://prometheus-kube-prometheus-prometheus.monitoring:9090)
#
# Installs the WVA controller from the official llm-d OCI registry.
# WVA watches VariantAutoscaling CRs and emits wva_desired_replicas metrics.

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

check_cli_exist helm

# VARIABLES
WVA_NAMESPACE="${WVA_NAMESPACE:-wva-system}"
WVA_RELEASE_NAME="${WVA_RELEASE_NAME:-llm-d-wva}"
WVA_PROMETHEUS_URL="${WVA_PROMETHEUS_URL:-https://prometheus-kube-prometheus-prometheus.monitoring:9090}"
# VARIABLES END

uninstall() {
    log_info "Uninstalling WVA..."

    helm uninstall "${WVA_RELEASE_NAME}" -n "${WVA_NAMESPACE}" 2>/dev/null || true
    kubectl delete all --all -n "${WVA_NAMESPACE}" --force --grace-period=0 2>/dev/null || true
    kubectl delete namespace "${WVA_NAMESPACE}" --wait=true --timeout=60s --force --grace-period=0 2>/dev/null || true

    log_success "WVA uninstalled"
}

install() {
    if helm list -n "${WVA_NAMESPACE}" 2>/dev/null | grep -q "${WVA_RELEASE_NAME}"; then
        if [ "$REINSTALL" = false ]; then
            log_info "WVA is already installed. Use --reinstall to reinstall."
            return 0
        else
            log_info "Reinstalling WVA..."
            uninstall
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

if [ "$UNINSTALL" = true ]; then
    uninstall
    exit 0
fi

install
