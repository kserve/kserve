#!/usr/bin/env bash
# Builds the vLLM CPU + NIXL image, smoke-tests it, fills the model PVC, deploys the
# P/D LLMInferenceService and runs validate.sh. Needs a cluster with the llmisvc
# controller, its LLMInferenceServiceConfig presets and a Gateway - see README.md.
#
# Usage:
#   MINIKUBE_PROFILE=<profile> ./run.sh      # build straight into a minikube node
#   VLLM_NIXL_CPU_IMAGE=<registry>/<repo>:<tag> PUSH=true ./run.sh
#
# Environment variables:
#   MINIKUBE_PROFILE     - build into this minikube profile's Docker daemon; also the
#                          default KUBE_CONTEXT
#   KUBE_CONTEXT         - kubectl context (default: MINIKUBE_PROFILE, else current context)
#   NAMESPACE            - namespace for the sample (default: pd-nixl-cpu)
#   VLLM_NIXL_CPU_IMAGE  - image to build and deploy
#                          (default: kserve-dev/vllm-openai-cpu-nixl:v0.19.0-nixl0.10.1[-noonednn])
#   DISABLE_ONEDNN_GEMM  - true on Apple silicon (the default there), false elsewhere
#   PUSH                 - true pushes the image after building (default: false)
#   SKIP_BUILD           - true reuses an existing image (default: false)
#   SKIP_SMOKE_TEST      - true skips the local docker run check (default: false)
set -euo pipefail
cd "$(dirname "$0")"

MINIKUBE_PROFILE="${MINIKUBE_PROFILE:-}"
KUBE_CONTEXT="${KUBE_CONTEXT:-$MINIKUBE_PROFILE}"
NAMESPACE="${NAMESPACE:-pd-nixl-cpu}"
PUSH="${PUSH:-false}"
SKIP_BUILD="${SKIP_BUILD:-false}"
SKIP_SMOKE_TEST="${SKIP_SMOKE_TEST:-false}"
SERVICE=opt-125m-pd-nixl-cpu

if [ -z "${DISABLE_ONEDNN_GEMM:-}" ]; then
    DISABLE_ONEDNN_GEMM=false
    if [ "$(uname -s)" = Darwin ] && [ "$(uname -m)" = arm64 ]; then
        DISABLE_ONEDNN_GEMM=true
    fi
fi
DEFAULT_IMAGE=kserve-dev/vllm-openai-cpu-nixl:v0.19.0-nixl0.10.1
[ "$DISABLE_ONEDNN_GEMM" = true ] && DEFAULT_IMAGE="${DEFAULT_IMAGE}-noonednn"
IMAGE="${VLLM_NIXL_CPU_IMAGE:-$DEFAULT_IMAGE}"

k() {
    if [ -n "$KUBE_CONTEXT" ]; then
        kubectl --context "$KUBE_CONTEXT" -n "$NAMESPACE" "$@"
    else
        kubectl -n "$NAMESPACE" "$@"
    fi
}

# Applies a sample manifest with the image placeholder filled in.
apply() { sed "s|\${VLLM_NIXL_CPU_IMAGE}|${IMAGE}|g" "$1" | k apply -f -; }

step() { printf '\n==> %s\n' "$*"; }

if [ -n "$MINIKUBE_PROFILE" ]; then
    # Build and smoke-test against the node's daemon, so the pods find the image there.
    eval "$(minikube -p "$MINIKUBE_PROFILE" docker-env)"
fi

if [ "$SKIP_BUILD" != true ]; then
    step "Building $IMAGE (DISABLE_ONEDNN_GEMM=$DISABLE_ONEDNN_GEMM)"
    docker build --build-arg "DISABLE_ONEDNN_GEMM=$DISABLE_ONEDNN_GEMM" -t "$IMAGE" .
    if [ "$PUSH" = true ]; then
        docker push "$IMAGE"
    fi
fi

if [ "$SKIP_SMOKE_TEST" != true ]; then
    # About a minute: catches a runtime that cannot start on this machine (Apple silicon's
    # oneDNN SIGILL, missing NIXL) before any cluster work. Dummy weights skip the
    # weight download; vLLM still fetches the config and tokenizer.
    step "Smoke-testing $IMAGE with NixlConnector and dummy weights"
    smoke=pd-nixl-cpu-smoke-$$
    docker run -d --name "$smoke" \
        -e VLLM_CPU_KVCACHE_SPACE=1 -e VLLM_CPU_OMP_THREADS_BIND=nobind -e UCX_TLS=tcp,self \
        -e PYTHONFAULTHANDLER=1 \
        --entrypoint vllm "$IMAGE" serve facebook/opt-125m --load-format dummy --port 8000 \
        --kv-transfer-config '{"kv_connector":"NixlConnector","kv_role":"kv_both","kv_buffer_device":"cpu"}' \
        >/dev/null
    result=timeout
    for _ in $(seq 1 120); do
        logs=$(docker logs "$smoke" 2>&1 || true)
        case "$logs" in
            *"Application startup complete"*) result=ok ;;
            *"Fatal Python error"* | *"Engine core initialization failed"* | *Traceback*) result=failed ;;
        esac
        [ "$result" != timeout ] && break
        [ "$(docker inspect -f '{{.State.Running}}' "$smoke")" = true ] || { result=failed; break; }
        sleep 5
    done
    if [ "$result" != ok ]; then
        # The root cause (a fault-handler stack or the engine's traceback) comes well
        # before the API server's own traceback, so print it first.
        docker logs "$smoke" 2>&1 | grep -v -E ' DEBUG |NVML' |
            awk '/Fatal Python error|Traceback/ && !seen { seen = 1 } seen && n < 25 { print; n++ }'
        echo "..."
        docker logs "$smoke" 2>&1 | grep -v -E ' DEBUG |NVML' | tail -5
        docker rm -f "$smoke" >/dev/null
        echo "Smoke test ${result}: the image cannot serve on this machine. See README.md, Workarounds." >&2
        exit 1
    fi
    docker logs "$smoke" 2>&1 | grep -E 'NIXL is available|Registering KV_Caches' | head -2
    docker rm -f "$smoke" >/dev/null
    echo "Smoke test passed."
fi

step "Preparing namespace $NAMESPACE"
k get namespace "$NAMESPACE" >/dev/null 2>&1 || k create namespace "$NAMESPACE"

step "Filling the model PVC"
apply model-pvc.yaml
if ! k wait --for=condition=Complete job/opt-125m-model-download --timeout=1800s; then
    k logs job/opt-125m-model-download --tail=30 || true
    exit 1
fi
k logs job/opt-125m-model-download | tail -n 8

step "Deploying $SERVICE"
apply llm-inference-service-pd-nixl-cpu.yaml
# CPU warm-up (torch compile) takes a few minutes per pod.
if ! k wait --for=condition=Ready "llminferenceservice/$SERVICE" --timeout=1200s; then
    k get pods
    k get llminferenceservice "$SERVICE" \
        -o jsonpath='{range .status.conditions[*]}{.type}={.status} {.reason}: {.message}{"\n"}{end}'
    exit 1
fi

step "Validating"
NAMESPACE="$NAMESPACE" SERVICE="$SERVICE" KUBE_CONTEXT="$KUBE_CONTEXT" VLLM_NIXL_CPU_IMAGE="$IMAGE" ./validate.sh
