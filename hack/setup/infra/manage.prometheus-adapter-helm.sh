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

# Install Prometheus Adapter using Helm
# Usage: manage.prometheus-adapter-helm.sh [--reinstall|--uninstall]
#   or:  REINSTALL=true manage.prometheus-adapter-helm.sh
#   or:  UNINSTALL=true manage.prometheus-adapter-helm.sh
#
# Environment variables for custom Helm values:
#   PROMETHEUS_ADAPTER_EXTRA_ARGS - Additional helm install arguments
#   PROMETHEUS_URL - Prometheus server URL (default: http://prometheus-kube-prometheus-prometheus.monitoring:9090)
#
# Configures the adapter to expose the wva_desired_replicas metric
# via the Kubernetes external metrics API for HPA consumption.

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
PROMETHEUS_ADAPTER_NAMESPACE="${PROMETHEUS_ADAPTER_NAMESPACE:-monitoring}"
PROMETHEUS_URL="${PROMETHEUS_URL:-https://prometheus-kube-prometheus-prometheus.monitoring}"
# VARIABLES END

uninstall() {
    log_info "Uninstalling Prometheus Adapter..."

    helm uninstall prometheus-adapter -n "${PROMETHEUS_ADAPTER_NAMESPACE}" 2>/dev/null || true

    log_success "Prometheus Adapter uninstalled"
}

install() {
    if helm list -n "${PROMETHEUS_ADAPTER_NAMESPACE}" 2>/dev/null | grep -q "prometheus-adapter"; then
        if [ "$REINSTALL" = false ]; then
            log_info "Prometheus Adapter is already installed. Use --reinstall to reinstall."
            return 0
        else
            log_info "Reinstalling Prometheus Adapter..."
            uninstall
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
        --set tls.enable=true \
        --set tls.ca="" \
        --set extraArguments[0]="--prometheus-auth-config={}" \
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

if [ "$UNINSTALL" = true ]; then
    uninstall
    exit 0
fi

install
