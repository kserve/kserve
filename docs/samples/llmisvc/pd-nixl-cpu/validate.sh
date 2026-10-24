#!/usr/bin/env bash
# Checks that a P/D LLMInferenceService really disaggregates: a request through the
# gateway must reach the prefill pod, and the decode pod must receive the prompt's KV
# over NIXL instead of recomputing it. A second request sent straight to the decode
# pod's routing sidecar (no scheduler, so no prefill) is the control.
#
# Usage:
#   ./validate.sh
#
# Environment variables:
#   NAMESPACE           - namespace of the service (default: pd-nixl-cpu)
#   SERVICE             - LLMInferenceService name (default: opt-125m-pd-nixl-cpu)
#   KUBE_CONTEXT        - kubectl context (default: current context)
#   VLLM_NIXL_CPU_IMAGE - image for the in-cluster client pod; any image with curl works
#                         (default: kserve-dev/vllm-openai-cpu-nixl:v0.19.0-nixl0.10.1)
#   GATEWAY_URL         - base URL of the service as seen from inside the cluster
#                         (default: the service's status.url)
#   EXPECT_KV_TRANSFER  - false checks only that prefill is reached, for images
#                         without NIXL (default: true)
set -euo pipefail

NAMESPACE="${NAMESPACE:-pd-nixl-cpu}"
SERVICE="${SERVICE:-opt-125m-pd-nixl-cpu}"
KUBE_CONTEXT="${KUBE_CONTEXT:-}"
IMAGE="${VLLM_NIXL_CPU_IMAGE:-kserve-dev/vllm-openai-cpu-nixl:v0.19.0-nixl0.10.1}"
EXPECT_KV_TRANSFER="${EXPECT_KV_TRANSFER:-true}"
CLIENT=pd-nixl-client
MODEL=facebook/opt-125m

k() {
    if [ -n "$KUBE_CONTEXT" ]; then
        kubectl --context "$KUBE_CONTEXT" -n "$NAMESPACE" "$@"
    else
        kubectl -n "$NAMESPACE" "$@"
    fi
}

# curl from inside the cluster, where pod IPs and the gateway are reachable.
kcurl() { k exec "$CLIENT" -- curl -sk --max-time 300 "$@"; }

if ! k get pod "$CLIENT" >/dev/null 2>&1; then
    echo "Creating client pod $CLIENT ..."
    k run "$CLIENT" --image "$IMAGE" --image-pull-policy IfNotPresent --restart Never \
        --command -- sleep infinity >/dev/null
fi
k wait --for=condition=Ready "pod/$CLIENT" --timeout=180s >/dev/null

pod_ip() {
    k get pod -l "app.kubernetes.io/name=${SERVICE},llm-d.ai/role=$1" \
        --field-selector=status.phase=Running -o jsonpath='{.items[0].status.podIP}'
}
P_IP=$(pod_ip prefill)
D_IP=$(pod_ip decode)

# The presets serve HTTPS when KServe's TLS is enabled, plain HTTP otherwise.
base_url() {
    if [ "$(kcurl -o /dev/null -w '%{http_code}' "https://$1/health" 2>/dev/null)" = 200 ]; then
        echo "https://$1"
    else
        echo "http://$1"
    fi
}
PREFILL=$(base_url "$P_IP:8000")   # prefill vLLM
DECODE=$(base_url "$D_IP:8001")    # decode vLLM, behind the sidecar
SIDECAR=$(base_url "$D_IP:8000")   # decode pod's routing sidecar
GATEWAY_URL="${GATEWAY_URL:-$(k get llminferenceservice "$SERVICE" -o jsonpath='{.status.url}')}"

# Sum of a vLLM counter over all label sets, optionally only those containing $3.
metric() {
    local body
    if ! body=$(kcurl "$1/metrics" 2>/dev/null); then
        echo "ERROR: cannot read $1/metrics from pod $CLIENT" >&2
        exit 1
    fi
    printf '%s\n' "$body" | awk -v m="$2" -v f="${3:-}" '
        /^#/ { next }
        { name = $1; sub(/\{.*/, "", name) }
        name == m && (f == "" || index($1, f)) { s += $2 }
        END { printf "%d", s }'
}

snapshot() {
    PREFILL_REQS=$(metric "$PREFILL" vllm:request_success_total)
    KV_TOKENS=$(metric "$DECODE" vllm:prompt_tokens_by_source_total 'source="external_kv_transfer"')
    LOCAL_TOKENS=$(metric "$DECODE" vllm:prompt_tokens_by_source_total 'source="local_compute"')
    NIXL_XFERS=$(metric "$DECODE" vllm:nixl_bytes_transferred_count)
    NIXL_BYTES=$(metric "$DECODE" vllm:nixl_bytes_transferred_sum)
    NIXL_FAILED=$(metric "$DECODE" vllm:nixl_num_failed_transfers_total)
}

PROMPT="You are an expert in Kubernetes-native machine learning serving platforms, with deep knowledge of the KServe project. Explain the challenges of serving large-scale models, GPU scheduling, and how KServe integrates with capabilities like multi-model serving. Provide a detailed comparison with open source alternatives, focusing on operational trade-offs."

# A unique prefix per request keeps decode's local prefix cache out of the picture.
# Prints the HTTP status; 000 means the endpoint could not be reached at all.
complete() {
    kcurl -o /dev/null -w '%{http_code}' -H 'Content-Type: application/json' "$1/v1/completions" \
        -d "{\"model\":\"$MODEL\",\"prompt\":\"[$2-$RANDOM] $PROMPT\",\"max_tokens\":20}" 2>/dev/null || true
}

FAILURES=0
check() { # description, test arguments...
    local description=$1
    shift
    if test "$@"; then
        printf '  PASS  %s\n' "$description"
    else
        printf '  FAIL  %s\n' "$description"
        FAILURES=$((FAILURES + 1))
    fi
}

report() { # label, http code
    printf '%-24s HTTP %s | prefill requests %+d | decode prompt tokens: kv_transfer %+d, local_compute %+d | nixl: transfers %+d, bytes %+d, failed %+d\n' \
        "$1" "$2" "$D_PREFILL_REQS" "$D_KV_TOKENS" "$D_LOCAL_TOKENS" "$D_NIXL_XFERS" "$D_NIXL_BYTES" "$D_NIXL_FAILED"
}

# Runs one request and sets the D_* deltas.
measure() { # url, tag
    snapshot
    local p=$PREFILL_REQS kv=$KV_TOKENS loc=$LOCAL_TOKENS x=$NIXL_XFERS b=$NIXL_BYTES f=$NIXL_FAILED
    CODE=$(complete "$1" "$2")
    snapshot
    D_PREFILL_REQS=$((PREFILL_REQS - p))
    D_KV_TOKENS=$((KV_TOKENS - kv))
    D_LOCAL_TOKENS=$((LOCAL_TOKENS - loc))
    D_NIXL_XFERS=$((NIXL_XFERS - x))
    D_NIXL_BYTES=$((NIXL_BYTES - b))
    D_NIXL_FAILED=$((NIXL_FAILED - f))
}

echo "prefill: $PREFILL  decode: $DECODE  sidecar: $SIDECAR"
echo "gateway: $GATEWAY_URL"
echo

measure "$GATEWAY_URL" gateway
report "via gateway" "$CODE"
if [ "$CODE" = 000 ]; then
    echo "  The gateway did not answer. Check that the Gateway's Envoy pod is Ready, or set GATEWAY_URL."
fi
check "gateway request returns 200" "$CODE" = 200
check "gateway request reaches the prefill pod" "$D_PREFILL_REQS" -ge 1
if [ "$EXPECT_KV_TRANSFER" = true ]; then
    check "decode receives prompt tokens over KV transfer" "$D_KV_TOKENS" -ge 1
    check "NIXL completes a transfer" "$D_NIXL_XFERS" -ge 1
    check "no failed NIXL transfers" "$D_NIXL_FAILED" -eq 0
fi
echo

measure "$SIDECAR/$NAMESPACE/$SERVICE" direct
report "control: decode only" "$CODE"
check "direct request returns 200" "$CODE" = 200
check "direct request does not reach the prefill pod" "$D_PREFILL_REQS" -eq 0
check "decode computes the prompt itself" "$D_LOCAL_TOKENS" -ge 1
check "decode receives no KV" "$D_KV_TOKENS" -eq 0
echo

if [ "$FAILURES" -gt 0 ]; then
    echo "FAILED: $FAILURES check(s)"
    exit 1
fi
echo "PASSED"
