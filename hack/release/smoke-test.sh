#!/usr/bin/env bash
# Pre-release smoke test for KServe.
#
# Creates a kind cluster, installs KServe with local charts,
# deploys sample workloads, and verifies inference responses.
#
# Usage:
#   ./hack/release/smoke-test.sh [OPTIONS]
#
# Options:
#   --skip-cluster-create   Skip kind cluster creation (use existing cluster)
#   --skip-cluster-delete   Keep kind cluster after test
#   --skip-llmisvc          Skip LLMIsvc test (ISVC only)
#   --namespace=NS          Target namespace (default: kserve)
#   --dry-run               Show what would be done without executing
#   -h, --help              Show this help message

set -eo pipefail

RED='\033[31m'
GREEN='\033[32m'
YELLOW='\033[33m'
BLUE='\033[34m'
NC='\033[0m'

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
DATA_DIR="$SCRIPT_DIR/smoke-test-data"

SKIP_CLUSTER_CREATE=false
SKIP_CLUSTER_DELETE=false
SKIP_LLMISVC=false
NAMESPACE="kserve"
DRY_RUN=false

ISVC_TIMEOUT=600      # 10 minutes
LLMISVC_TIMEOUT=1200  # 20 minutes
POLL_INTERVAL=30

usage() {
    sed -n '2,/^$/s/^# \{0,1\}//p' "$0"
    exit 0
}

for arg in "$@"; do
    case "$arg" in
        --skip-cluster-create) SKIP_CLUSTER_CREATE=true ;;
        --skip-cluster-delete) SKIP_CLUSTER_DELETE=true ;;
        --skip-llmisvc) SKIP_LLMISVC=true ;;
        --namespace=*) NAMESPACE="${arg#--namespace=}" ;;
        --dry-run) DRY_RUN=true ;;
        -h|--help) usage ;;
        *) echo -e "${RED}Unknown argument: $arg${NC}"; usage ;;
    esac
done

print_step() {
    echo ""
    echo -e "${BLUE}==== $1 ====${NC}"
}

print_pass() {
    echo -e "${GREEN}✅ $1${NC}"
}

print_fail() {
    echo -e "${RED}❌ $1${NC}"
}

print_info() {
    echo -e "${YELLOW}$1${NC}"
}

cleanup_port_forward() {
    if [[ -n "${PF_PID:-}" ]]; then
        kill "$PF_PID" 2>/dev/null || true
        wait "$PF_PID" 2>/dev/null || true
        unset PF_PID
    fi
}

trap cleanup_port_forward EXIT

wait_for_ready() {
    local resource_type="$1"
    local resource_name="$2"
    local timeout="$3"
    local elapsed=0

    print_info "Waiting for $resource_type/$resource_name to be Ready (timeout: ${timeout}s)..."

    while [[ $elapsed -lt $timeout ]]; do
        local status
        status=$(kubectl get "$resource_type" "$resource_name" -n "$NAMESPACE" \
            -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null || echo "")

        if [[ "$status" == "True" ]]; then
            print_pass "$resource_type/$resource_name is Ready (${elapsed}s)"
            return 0
        fi

        sleep "$POLL_INTERVAL"
        elapsed=$((elapsed + POLL_INTERVAL))
        echo "  ... ${elapsed}s elapsed (status: ${status:-pending})"
    done

    print_fail "$resource_type/$resource_name not ready after ${timeout}s"
    kubectl get "$resource_type" "$resource_name" -n "$NAMESPACE" -o yaml 2>/dev/null || true
    return 1
}

# ============================================================
# Dry-run
# ============================================================

if [[ "$DRY_RUN" == "true" ]]; then
    echo ""
    echo -e "${BLUE}[DRY-RUN] Smoke test plan:${NC}"
    echo ""
    echo "  1. Create kind cluster (skip: $SKIP_CLUSTER_CREATE)"
    echo "  2. Install KServe with local charts"
    echo "     ./hack/kserve-install.sh --type kserve,localmodel,llmisvc --raw --local-chart"
    echo "  3. Deploy sklearn-iris ISVC"
    echo "     kubectl apply -f $DATA_DIR/sklearn-iris.yaml"
    echo "     → curl inference endpoint, verify predictions"
    if [[ "$SKIP_LLMISVC" == "false" ]]; then
        echo "  4. Deploy LLMIsvc (facebook-opt-125m)"
        echo "     kubectl apply -f $DATA_DIR/llmisvc-opt-125m-cpu.yaml"
        echo "     → curl /v1/completions endpoint, verify choices"
    else
        echo "  4. LLMIsvc test: SKIPPED"
    fi
    echo "  5. Delete kind cluster (skip: $SKIP_CLUSTER_DELETE)"
    echo ""
    echo -e "${GREEN}Dry-run complete. No changes made.${NC}"
    exit 0
fi

# ============================================================
# Step 1: Create kind cluster
# ============================================================

if [[ "$SKIP_CLUSTER_CREATE" == "false" ]]; then
    print_step "Step 1/5: Creating kind cluster"
    "$REPO_ROOT/hack/setup/dev/manage.kind-with-registry.sh"
    print_pass "Kind cluster created"
else
    print_step "Step 1/5: Skipping cluster creation (--skip-cluster-create)"
fi

# ============================================================
# Step 2: Install KServe with local charts
# ============================================================

print_step "Step 2/5: Installing KServe with local charts"
"$REPO_ROOT/hack/kserve-install.sh" --type kserve,localmodel,llmisvc --raw --local-chart
print_pass "KServe installed"

# ============================================================
# Step 3: Test ISVC (sklearn-iris)
# ============================================================

print_step "Step 3/5: Testing ISVC (sklearn-iris)"

kubectl apply -f "$DATA_DIR/sklearn-iris.yaml" -n "$NAMESPACE"

if ! wait_for_ready "isvc" "sklearn-iris" "$ISVC_TIMEOUT"; then
    print_fail "ISVC test failed: sklearn-iris did not become Ready"
    exit 1
fi

# Verify inference via curl
print_info "Verifying inference response..."
kubectl port-forward -n "$NAMESPACE" svc/sklearn-iris-predictor 8080:80 &
PF_PID=$!
sleep 3

RESPONSE=$(curl -s --max-time 10 localhost:8080/v1/models/sklearn-iris:predict \
    -H "Content-Type: application/json" \
    -d @"$DATA_DIR/sklearn-iris-input.json" 2>/dev/null || echo "")

cleanup_port_forward

if echo "$RESPONSE" | grep -q '"predictions"'; then
    print_pass "sklearn-iris inference verified: $RESPONSE"
else
    print_fail "sklearn-iris inference failed. Response: $RESPONSE"
    kubectl delete isvc sklearn-iris -n "$NAMESPACE" --ignore-not-found
    exit 1
fi

# Clean up ISVC
kubectl delete isvc sklearn-iris -n "$NAMESPACE"
kubectl wait --for=delete pod -l serving.kserve.io/inferenceservice=sklearn-iris \
    -n "$NAMESPACE" --timeout=120s 2>/dev/null || true
print_pass "sklearn-iris cleaned up"

# ============================================================
# Step 4: Test LLMIsvc (facebook-opt-125m)
# ============================================================

if [[ "$SKIP_LLMISVC" == "true" ]]; then
    print_step "Step 4/5: Skipping LLMIsvc test (--skip-llmisvc)"
else
    print_step "Step 4/5: Testing LLMIsvc (facebook-opt-125m)"

    kubectl apply -f "$DATA_DIR/llmisvc-opt-125m-cpu.yaml" -n "$NAMESPACE"

    if ! wait_for_ready "llmisvc" "facebook-opt-125m-single" "$LLMISVC_TIMEOUT"; then
        print_fail "LLMIsvc test failed: facebook-opt-125m-single did not become Ready"
        kubectl delete llmisvc facebook-opt-125m-single -n "$NAMESPACE" --ignore-not-found
        exit 1
    fi

    # Verify inference via curl (vLLM OpenAI-compatible endpoint on workload svc port 8000)
    print_info "Verifying LLMIsvc inference response..."
    kubectl port-forward -n "$NAMESPACE" svc/facebook-opt-125m-single-kserve-workload-svc 8081:8000 &
    PF_PID=$!
    sleep 3

    RESPONSE=$(curl -s --max-time 30 localhost:8081/v1/completions \
        -H "Content-Type: application/json" \
        -d @"$DATA_DIR/llmisvc-input.json" 2>/dev/null || echo "")

    cleanup_port_forward

    if echo "$RESPONSE" | grep -q '"choices"'; then
        print_pass "LLMIsvc inference verified: $(echo "$RESPONSE" | head -c 200)"
    else
        print_fail "LLMIsvc inference failed. Response: $RESPONSE"
        kubectl delete llmisvc facebook-opt-125m-single -n "$NAMESPACE" --ignore-not-found
        exit 1
    fi

    # Clean up LLMIsvc
    kubectl delete llmisvc facebook-opt-125m-single -n "$NAMESPACE"
    print_pass "facebook-opt-125m-single cleaned up"
fi

# ============================================================
# Step 5: Clean up kind cluster
# ============================================================

if [[ "$SKIP_CLUSTER_DELETE" == "false" ]]; then
    print_step "Step 5/5: Deleting kind cluster"
    "$REPO_ROOT/hack/setup/dev/manage.kind-with-registry.sh" --uninstall
    print_pass "Kind cluster deleted"
else
    print_step "Step 5/5: Skipping cluster deletion (--skip-cluster-delete)"
fi

echo ""
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}  Smoke test passed!${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""
