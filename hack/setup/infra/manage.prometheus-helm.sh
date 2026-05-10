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

# Install Prometheus (kube-prometheus-stack) using Helm
# Usage: manage.prometheus-helm.sh [--reinstall|--uninstall]
#   or:  REINSTALL=true manage.prometheus-helm.sh
#   or:  UNINSTALL=true manage.prometheus-helm.sh
#
# Environment variables for custom Helm values:
#   PROMETHEUS_EXTRA_ARGS - Additional helm install arguments
#
# Installs a minimal Prometheus stack suitable for CI:
# - Prometheus server + ServiceMonitor CRD
# - Grafana, AlertManager, and node-exporter disabled
# - TLS enabled via cert-manager self-signed certificate (required by WVA)

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
PROMETHEUS_NAMESPACE="${PROMETHEUS_NAMESPACE:-monitoring}"
PROMETHEUS_RELEASE_NAME="${PROMETHEUS_RELEASE_NAME:-prometheus}"
# VARIABLES END

uninstall() {
    log_info "Uninstalling Prometheus..."

    helm uninstall "${PROMETHEUS_RELEASE_NAME}" -n "${PROMETHEUS_NAMESPACE}" 2>/dev/null || true
    kubectl delete all --all -n "${PROMETHEUS_NAMESPACE}" --force --grace-period=0 2>/dev/null || true
    kubectl delete namespace "${PROMETHEUS_NAMESPACE}" --wait=true --timeout=60s --force --grace-period=0 2>/dev/null || true

    log_success "Prometheus uninstalled"
}

install() {
    if helm list -n "${PROMETHEUS_NAMESPACE}" 2>/dev/null | grep -q "${PROMETHEUS_RELEASE_NAME}"; then
        if [ "$REINSTALL" = false ]; then
            log_info "Prometheus is already installed. Use --reinstall to reinstall."
            return 0
        else
            log_info "Reinstalling Prometheus..."
            uninstall
        fi
    fi

    log_info "Adding prometheus-community Helm repository..."
    helm repo add prometheus-community https://prometheus-community.github.io/helm-charts --force-update

    log_info "Creating self-signed TLS certificate for Prometheus via cert-manager..."
    kubectl create namespace "${PROMETHEUS_NAMESPACE}" 2>/dev/null || true

    kubectl apply -f - <<EOF
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: prometheus-selfsigned
  namespace: ${PROMETHEUS_NAMESPACE}
spec:
  selfSigned: {}
---
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: prometheus-tls
  namespace: ${PROMETHEUS_NAMESPACE}
spec:
  secretName: prometheus-tls
  duration: 8760h
  issuerRef:
    name: prometheus-selfsigned
    kind: Issuer
  dnsNames:
    - ${PROMETHEUS_RELEASE_NAME}-kube-prometheus-prometheus.${PROMETHEUS_NAMESPACE}
    - ${PROMETHEUS_RELEASE_NAME}-kube-prometheus-prometheus.${PROMETHEUS_NAMESPACE}.svc
    - ${PROMETHEUS_RELEASE_NAME}-kube-prometheus-prometheus.${PROMETHEUS_NAMESPACE}.svc.cluster.local
EOF

    kubectl wait certificate prometheus-tls -n "${PROMETHEUS_NAMESPACE}" --for=condition=Ready --timeout=60s

    log_info "Installing kube-prometheus-stack ${PROMETHEUS_VERSION}..."
    helm install "${PROMETHEUS_RELEASE_NAME}" prometheus-community/kube-prometheus-stack \
        --namespace "${PROMETHEUS_NAMESPACE}" \
        --create-namespace \
        --version "${PROMETHEUS_VERSION}" \
        --set grafana.enabled=false \
        --set alertmanager.enabled=false \
        --set nodeExporter.enabled=false \
        --set kubeStateMetrics.enabled=false \
        --set prometheus.prometheusSpec.resources.requests.cpu=50m \
        --set prometheus.prometheusSpec.resources.requests.memory=256Mi \
        --set prometheus.prometheusSpec.resources.limits.cpu=200m \
        --set prometheus.prometheusSpec.resources.limits.memory=512Mi \
        --set prometheus.prometheusSpec.web.tlsConfig.cert.secret.name=prometheus-tls \
        --set prometheus.prometheusSpec.web.tlsConfig.cert.secret.key=tls.crt \
        --set prometheus.prometheusSpec.web.tlsConfig.keySecret.name=prometheus-tls \
        --set prometheus.prometheusSpec.web.tlsConfig.keySecret.key=tls.key \
        --wait \
        --timeout 10m \
        ${PROMETHEUS_EXTRA_ARGS:-}

    log_success "Successfully installed kube-prometheus-stack ${PROMETHEUS_VERSION} via Helm"

    wait_for_pods "${PROMETHEUS_NAMESPACE}" "app.kubernetes.io/name=prometheus" "300s"

    log_success "Prometheus is ready (TLS enabled)!"
}

if [ "$UNINSTALL" = true ]; then
    uninstall
    exit 0
fi

install
