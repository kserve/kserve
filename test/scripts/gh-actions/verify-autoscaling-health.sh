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

# Fail-fast health check for the autoscaling metrics pipeline.
# Validates that Prometheus, WVA, and the external metrics API are wired
# correctly BEFORE running e2e tests. Exits non-zero on first failure.
#
# Usage: verify-autoscaling-health.sh <hpa|keda>

set -euo pipefail

AUTOSCALER="${1:?Usage: verify-autoscaling-health.sh <hpa|keda>}"

PROMETHEUS_NAMESPACE="${PROMETHEUS_NAMESPACE:-monitoring}"
WVA_NAMESPACE="${WVA_NAMESPACE:-wva-system}"
KEDA_NAMESPACE="${KEDA_NAMESPACE:-keda}"

retry() {
    local description="$1"
    local timeout="$2"
    local interval="${3:-5}"
    shift 3
    local cmd=("$@")

    local deadline=$((SECONDS + timeout))
    local attempt=0
    while true; do
        attempt=$((attempt + 1))
        if "${cmd[@]}" 2>/dev/null; then
            echo "  [PASS] ${description}"
            return 0
        fi
        if [ $SECONDS -ge $deadline ]; then
            echo "  [FAIL] ${description} (timed out after ${timeout}s, ${attempt} attempts)"
            return 1
        fi
        sleep "$interval"
    done
}

echo "======================================================================"
echo "Autoscaling Pipeline Health Check (mode: ${AUTOSCALER})"
echo "======================================================================"

# ---------------------------------------------------------------------------
# 1. Pod readiness
# ---------------------------------------------------------------------------
echo ""
echo "--- Step 1: Verifying pod readiness ---"

echo "  Waiting for Prometheus pods..."
kubectl wait --for=condition=Ready pod \
    -l app.kubernetes.io/name=prometheus \
    -n "${PROMETHEUS_NAMESPACE}" \
    --timeout=120s

echo "  Waiting for WVA pods..."
kubectl wait --for=condition=Ready pod \
    -l control-plane=controller-manager \
    -n "${WVA_NAMESPACE}" \
    --timeout=120s

if [[ "${AUTOSCALER}" == "hpa" ]]; then
    echo "  Waiting for Prometheus Adapter pods..."
    kubectl wait --for=condition=Ready pod \
        -l app.kubernetes.io/name=prometheus-adapter \
        -n "${PROMETHEUS_NAMESPACE}" \
        --timeout=120s
elif [[ "${AUTOSCALER}" == "keda" ]]; then
    echo "  Waiting for KEDA operator pods..."
    kubectl wait --for=condition=Ready pod \
        -l app.kubernetes.io/name=keda-operator \
        -n "${KEDA_NAMESPACE}" \
        --timeout=120s
fi

echo "  [PASS] All pods are Ready"

# ---------------------------------------------------------------------------
# 2. Prometheus API reachable
# ---------------------------------------------------------------------------
echo ""
echo "--- Step 2: Verifying Prometheus API is reachable ---"

PROM_POD=$(kubectl get pods -n "${PROMETHEUS_NAMESPACE}" \
    -l app.kubernetes.io/name=prometheus \
    -l app.kubernetes.io/managed-by=prometheus-operator \
    -o jsonpath='{.items[0].metadata.name}')

check_prometheus_api() {
    kubectl exec -n "${PROMETHEUS_NAMESPACE}" "${PROM_POD}" -c prometheus -- \
        wget -qO- --no-check-certificate \
        "https://localhost:9090/api/v1/status/config" | grep -q '"status":"success"'
}

retry "Prometheus API responds" 30 5 check_prometheus_api

# ---------------------------------------------------------------------------
# 3. WVA ServiceMonitor target is UP in Prometheus
# ---------------------------------------------------------------------------
echo ""
echo "--- Step 3: Verifying WVA ServiceMonitor target is scraped ---"

check_wva_target_up() {
    kubectl exec -n "${PROMETHEUS_NAMESPACE}" "${PROM_POD}" -c prometheus -- \
        wget -qO- --no-check-certificate \
        "https://localhost:9090/api/v1/targets" | grep -q "${WVA_NAMESPACE}"
}

retry "WVA target discovered by Prometheus" 60 5 check_wva_target_up

# ---------------------------------------------------------------------------
# 4. WVA controller can reach Prometheus (log smoke-check)
# ---------------------------------------------------------------------------
echo ""
echo "--- Step 4: Verifying WVA controller health (log check) ---"

WVA_POD=$(kubectl get pods -n "${WVA_NAMESPACE}" \
    -l control-plane=controller-manager \
    -o jsonpath='{.items[0].metadata.name}')

check_wva_no_prometheus_errors() {
    local logs
    logs=$(kubectl logs -n "${WVA_NAMESPACE}" "${WVA_POD}" --tail=50 2>/dev/null || echo "")
    if echo "${logs}" | grep -qi "error.*prometheus\|connection refused\|no such host\|dial tcp.*refused"; then
        return 1
    fi
    return 0
}

if ! check_wva_no_prometheus_errors; then
    echo "  [FAIL] WVA controller has Prometheus connectivity errors in logs:"
    kubectl logs -n "${WVA_NAMESPACE}" "${WVA_POD}" --tail=20
    exit 1
fi
echo "  [PASS] WVA controller logs show no Prometheus errors"

# ---------------------------------------------------------------------------
# 5. External Metrics API is healthy
# ---------------------------------------------------------------------------
echo ""
echo "--- Step 5: Verifying External Metrics API ---"

check_external_metrics_api() {
    kubectl get --raw /apis/external.metrics.k8s.io/v1beta1 >/dev/null 2>&1
}

retry "External Metrics API discovery endpoint" 60 5 check_external_metrics_api

# ---------------------------------------------------------------------------
# 5b. Autoscaler-specific checks
# ---------------------------------------------------------------------------
if [[ "${AUTOSCALER}" == "hpa" ]]; then
    echo ""
    echo "--- Step 5b: Verifying Prometheus Adapter APIService ---"

    check_apiservice_available() {
        local status
        status=$(kubectl get apiservice v1beta1.external.metrics.k8s.io \
            -o jsonpath='{.status.conditions[?(@.type=="Available")].status}')
        [[ "${status}" == "True" ]]
    }

    retry "APIService v1beta1.external.metrics.k8s.io Available" 60 5 check_apiservice_available

elif [[ "${AUTOSCALER}" == "keda" ]]; then
    echo ""
    echo "--- Step 5b: Verifying KEDA metrics server ---"

    check_keda_metrics_server() {
        kubectl get pods -n "${KEDA_NAMESPACE}" \
            -l app=keda-operator-metrics-apiserver \
            -o jsonpath='{.items[0].status.phase}' | grep -q "Running"
    }

    retry "KEDA metrics server Running" 60 5 check_keda_metrics_server
fi

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
echo ""
echo "======================================================================"
echo "All autoscaling pipeline health checks PASSED (mode: ${AUTOSCALER})"
echo "======================================================================"
