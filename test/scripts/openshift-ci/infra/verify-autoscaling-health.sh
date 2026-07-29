#!/usr/bin/env bash
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
#
# Fail-fast health check for the KEDA autoscaling metrics pipeline on OpenShift.
#
# Validates that User Workload Monitoring (UWM), WVA, KEDA, and the
# ClusterTriggerAuthentication are wired correctly BEFORE running e2e tests.
# Exits non-zero on first failure.
#
# Metrics pipeline:
#   simulator pod → PodMonitor → UWM Prometheus → Thanos Querier ← WVA
#                                                                 ← KEDA (via ScaledObject)
#
# Usage: verify-autoscaling-health.sh

set -euo pipefail

: "${KEDA_NAMESPACE:=openshift-keda}"
: "${WVA_NAMESPACE:=wva-system}"
WVA_DEPLOY="workload-variant-autoscaler-controller-manager"

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
echo "KEDA Autoscaling Pipeline Health Check (OpenShift)"
echo "======================================================================"

# ---------------------------------------------------------------------------
# 1. User Workload Monitoring
# ---------------------------------------------------------------------------
echo ""
echo "--- Step 1: Verifying User Workload Monitoring ---"

echo "  Checking UWM Prometheus pods..."
oc wait --for=condition=Ready pod \
    -l app.kubernetes.io/name=prometheus \
    -n openshift-user-workload-monitoring \
    --timeout=120s
echo "  [PASS] UWM Prometheus pods are Ready"

echo "  Checking Thanos Querier pods..."
oc wait --for=condition=Ready pod \
    -l app.kubernetes.io/name=thanos-query \
    -n openshift-monitoring \
    --timeout=120s
echo "  [PASS] Thanos Querier pods are Ready"

# ---------------------------------------------------------------------------
# 2. WVA controller
# ---------------------------------------------------------------------------
echo ""
echo "--- Step 2: Verifying WVA controller ---"

echo "  Checking WVA pods..."
oc wait --for=condition=Ready pod \
    -l control-plane=controller-manager \
    -n "${WVA_NAMESPACE}" \
    --timeout=120s
echo "  [PASS] WVA controller pod is Ready"

check_wva_no_errors() {
    local logs
    if ! logs=$(oc logs -n "${WVA_NAMESPACE}" \
        deployment/"${WVA_DEPLOY}" --tail=50 2>/dev/null); then
        return 1
    fi
    if echo "${logs}" | grep -qi "error.*prometheus\|connection refused\|no such host\|x509.*certificate"; then
        return 1
    fi
    return 0
}
retry "WVA logs show no Prometheus/TLS errors" 30 5 check_wva_no_errors

# ---------------------------------------------------------------------------
# 3. KEDA operator
# ---------------------------------------------------------------------------
echo ""
echo "--- Step 3: Verifying KEDA/CMA operator ---"

check_keda_operator() {
    for selector in "app=keda-operator" "app.kubernetes.io/name=keda-operator"; do
        if oc wait --for=condition=Ready pod \
            -l "${selector}" -n "${KEDA_NAMESPACE}" \
            --timeout=0s 2>/dev/null; then
            return 0
        fi
    done
    return 1
}
retry "KEDA operator pod is Ready" 60 5 check_keda_operator

check_keda_metrics_server() {
    for selector in "app=keda-metrics-apiserver" "app.kubernetes.io/name=keda-operator-metrics-apiserver"; do
        if oc wait --for=condition=Ready pod \
            -l "${selector}" -n "${KEDA_NAMESPACE}" \
            --timeout=0s 2>/dev/null; then
            return 0
        fi
    done
    return 1
}
retry "KEDA metrics API server is Ready" 60 5 check_keda_metrics_server

# ---------------------------------------------------------------------------
# 4. ClusterTriggerAuthentication
# ---------------------------------------------------------------------------
echo ""
echo "--- Step 4: Verifying KEDA → Thanos authentication ---"

check_cluster_trigger_auth() {
    oc get clustertriggerauthentication ai-inference-keda-thanos \
        -o jsonpath='{.metadata.name}' | grep -q "ai-inference-keda-thanos"
}
retry "ClusterTriggerAuthentication ai-inference-keda-thanos exists" 10 2 check_cluster_trigger_auth

check_keda_sa_token() {
    local token
    token=$(oc get secret keda-operator-token -n "${KEDA_NAMESPACE}" \
        -o jsonpath='{.data.token}' 2>/dev/null)
    [[ -n "$token" ]]
}
retry "KEDA SA token secret is populated" 30 5 check_keda_sa_token

check_keda_monitoring_rbac() {
    oc get clusterrolebinding keda-thanos-sa-monitoring-view \
        -o jsonpath='{.roleRef.name}' 2>/dev/null | grep -q "cluster-monitoring-view"
}
retry "KEDA SA has cluster-monitoring-view role" 10 2 check_keda_monitoring_rbac

# ---------------------------------------------------------------------------
# 5. External Metrics API
# ---------------------------------------------------------------------------
echo ""
echo "--- Step 5: Verifying External Metrics API ---"

check_external_metrics_api() {
    oc get --raw /apis/external.metrics.k8s.io/v1beta1 >/dev/null 2>&1
}
retry "External Metrics API discovery endpoint" 60 5 check_external_metrics_api

# ---------------------------------------------------------------------------
# 6. WVA ServiceMonitor (so wva_desired_replicas is scraped)
# ---------------------------------------------------------------------------
echo ""
echo "--- Step 6: Verifying WVA metrics scraping ---"

check_wva_servicemonitor() {
    oc get servicemonitor -n "${WVA_NAMESPACE}" -o name 2>/dev/null | grep -q "servicemonitor"
}
retry "WVA ServiceMonitor exists" 30 5 check_wva_servicemonitor

# ---------------------------------------------------------------------------
# 7. inferenceservice-config has autoscaling-wva-controller-config
# ---------------------------------------------------------------------------
echo ""
echo "--- Step 7: Verifying inferenceservice-config ---"

: "${KSERVE_NAMESPACE:=kserve}"

check_wva_config() {
    local config
    config=$(oc get configmap inferenceservice-config -n "${KSERVE_NAMESPACE}" \
        -o jsonpath='{.data.autoscaling-wva-controller-config}' 2>/dev/null)
    echo "$config" | grep -q "ai-inference-keda-thanos"
}
retry "autoscaling-wva-controller-config references ClusterTriggerAuthentication" 10 2 check_wva_config

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
echo ""
echo "======================================================================"
echo "All KEDA autoscaling pipeline health checks PASSED"
echo "======================================================================"
