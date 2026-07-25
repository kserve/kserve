# Agentic Tool Calling — E2E Test Guide

This guide validates that tool calling parameters pass through the full KServe LLMInferenceService serving stack unaltered, on both the **OpenAI Chat Completions API** (`/v1/chat/completions`) and the **Anthropic Messages API** (`/v1/messages`).

The request path under test:

```
Client → Gateway → HTTPRoute → InferencePool → EPP → pd-sidecar → vLLM
```

## What This Tests

- Tool definitions (`tools` array) arrive at the model server intact
- Tool call responses (`tool_calls` / `tool_use`) reach the client unaltered
- Streaming tool call deltas assemble correctly
- Multi-turn tool use (tool results in conversation history) works
- Multiple tool definitions and parallel tool calls are preserved
- `tool_choice` field (auto, specific function) passes through
- Complex nested parameter schemas survive serialization
- Non-tool-calling requests are not degraded

## Deployment Options

Two LLMInferenceService manifests are provided:

| Manifest | Model | GPUs | Topology | Use Case |
|----------|-------|------|----------|----------|
| [`nemotron-3-ultra-pd.yaml`](nemotron-3-ultra-pd.yaml) | Nemotron-3-Ultra-550B | 2x 8-GPU nodes | P/D disaggregated | Production reference, tests pd-sidecar passthrough |
| [`llmisvc-single-gpu.yaml`](llmisvc-single-gpu.yaml) | Qwen3-Coder-30B-A3B-Instruct-FP8 | 1 GPU | Single node | Fast iteration, CI testing |

## Prerequisites

- KServe with LLMInferenceService controller installed ([quick-install script](../../../../hack/setup/quick-install/llmisvc-full-install-with-manifests.sh))
- Gateway API Inference Extension CRDs (v1 + v1alpha2 InferencePool)
- `kubectl`, `jq`, `curl`
- HuggingFace token with model access

### Known Issues

1. **v1alpha2 InferencePool CRD**: The quick-install script may not install the `inferencepools.inference.networking.x-k8s.io` CRD, causing the controller to fail. Fix: [kserve#5751](https://github.com/kserve/kserve/pull/5751).

2. **Hybrid model prefix caching**: Nemotron-3-Ultra (Mamba+Attention hybrid) requires `--enable-prefix-caching` for KV offloading. Without it, vLLM fails with `gpu_block_size not divisible by hash_block_size`.

3. **Storage-initializer memory**: The default 1Gi memory limit for the storage-initializer is too small for large models (~530GB for Nemotron). Increase it:
   ```bash
   kubectl get configmap inferenceservice-config -n kserve -o json | \
     python3 -c "
   import json, sys
   cm = json.load(sys.stdin)
   si = json.loads(cm['data']['storageInitializer'])
   si['memoryLimit'] = '8Gi'
   si['memoryRequest'] = '2Gi'
   cm['data']['storageInitializer'] = json.dumps(si)
   json.dump(cm, sys.stdout)
   " | kubectl apply -f -
   ```

## Setup

```bash
export NAMESPACE=tool-calling-test
export MODEL=<model-name>  # e.g., Qwen/Qwen3-Coder-30B-A3B-Instruct-FP8

kubectl create namespace ${NAMESPACE}
kubectl create secret generic hf-token \
  --from-literal="HF_TOKEN=${HF_TOKEN}" \
  --namespace "${NAMESPACE}"
```

## Deploy

### Option A: Single GPU (Qwen3-Coder-30B)

```bash
kubectl apply -n ${NAMESPACE} -f llmisvc-single-gpu.yaml
kubectl wait --for=condition=Ready llminferenceservice/qwen3-coder-tool-calling \
  -n ${NAMESPACE} --timeout=1800s
```

### Option B: P/D Disaggregated (Nemotron-3-Ultra-550B)

```bash
kubectl apply -n ${NAMESPACE} -f nemotron-3-ultra-pd.yaml
kubectl rollout status deployment/nemotron-ultra-pd-kserve -n ${NAMESPACE} --timeout=3600s
kubectl rollout status deployment/nemotron-ultra-pd-kserve-prefill -n ${NAMESPACE} --timeout=3600s
```

## Get the Endpoint

### Via Gateway (full stack path — recommended)

```bash
GATEWAY_IP=$(kubectl get gateway kserve-ingress-gateway -n kserve \
  -o jsonpath='{.status.addresses[0].value}')
export BASE_URL="http://${GATEWAY_IP}/${NAMESPACE}/<llmisvc-name>"
```

To access from outside the cluster, port-forward the Gateway's Envoy proxy:

```bash
kubectl port-forward -n envoy-gateway-system \
  svc/$(kubectl get svc -n envoy-gateway-system -l gateway.envoyproxy.io/owning-gateway-name=kserve-ingress-gateway -o name | head -1 | sed 's|service/||') \
  8080:80 &
export BASE_URL="http://localhost:8080/${NAMESPACE}/<llmisvc-name>"
```

### Via workload service (bypasses Gateway/EPP — for debugging)

```bash
kubectl port-forward -n ${NAMESPACE} svc/<llmisvc-name>-kserve-workload-svc 8000:8000 &
export BASE_URL="http://localhost:8000"
```

## Run Tests

### Automated (all 19 tests)

```bash
MODEL=${MODEL} LLMISVC_NAME=<llmisvc-name> NAMESPACE=${NAMESPACE} ./run-tests.sh
```

Or pass the base URL directly:

```bash
./run-tests.sh "${BASE_URL}"
```

For full response body logging:

```bash
VERBOSE=true MODEL=${MODEL} LLMISVC_NAME=<llmisvc-name> ./run-tests.sh 2>&1 | tee test-results.log
```

### Manual (individual requests)

See [`test-requests.md`](test-requests.md) for all curl commands with expected responses.

Quick smoke test:

```bash
curl -s -X POST ${BASE_URL}/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "'${MODEL}'",
    "temperature": 0,
    "messages": [
      {"role": "user", "content": "What is the weather in San Francisco?"}
    ],
    "tools": [{
      "type": "function",
      "function": {
        "name": "get_weather",
        "description": "Get weather for a location",
        "parameters": {
          "type": "object",
          "properties": {"location": {"type": "string"}},
          "required": ["location"]
        }
      }
    }],
    "tool_choice": "auto"
  }' | jq '.choices[0].message.tool_calls'
```

## Verify Stack Passthrough

The LLMInferenceService manifests include `--enable-log-requests` and `--enable-log-outputs` flags. After running tests, check vLLM logs to verify tool calling parameters arrived at the model server and tool call responses were produced:

```bash
# Decode pod — check response contains tool calls
kubectl logs -n ${NAMESPACE} -l app.kubernetes.io/name=<llmisvc-name>,kserve.io/component=workload \
  -c main --tail=100 | grep -i "Generated response\|tool_calls\|tool_use\|get_weather"

# Prefill pod (P/D mode only) — check request arrived
kubectl logs -n ${NAMESPACE} -l app.kubernetes.io/name=<llmisvc-name>,kserve.io/component=workload \
  -c main --tail=100 | grep -i "Received request"

# pd-sidecar (P/D mode only) — check request forwarding
kubectl logs -n ${NAMESPACE} -l app.kubernetes.io/name=<llmisvc-name>,kserve.io/component=workload \
  -c llm-d-routing-sidecar --tail=100 | grep -i "forward\|error"
```

## Test Matrix

| # | Test | API | What it validates |
|---|------|-----|-------------------|
| 1 | Tool calling (non-streaming) | Chat Completions | Basic tool call passthrough |
| 2 | Tool calling (streaming) | Chat Completions | SSE delta assembly |
| 3 | Tool calling (non-streaming) | Messages API | Anthropic format passthrough |
| 4 | Tool calling (streaming) | Messages API | Anthropic SSE events |
| 5 | Multi-turn tool use | Chat Completions | Tool result in message history |
| 6 | Multi-turn tool use | Messages API | tool_result block passthrough |
| 7 | Multiple tools (2 definitions) | Chat Completions | Multiple tool defs preserved |
| 8 | Multiple tools (2 definitions) | Messages API | Multiple tool defs preserved |
| 9 | 5 tools, use all | Chat Completions | Large tools array not truncated |
| 10 | 5 tools, use all | Messages API | Large tools array not truncated |
| 11 | tool_choice specific function | Chat Completions | tool_choice field preserved |
| 12 | tool_choice specific tool | Messages API | Anthropic tool_choice preserved |
| 13 | Complex nested parameters | Chat Completions | Nested objects/arrays/enums in schema |
| 14 | Tool with no parameters | Chat Completions | Empty params edge case |
| 15 | Multiple calls same tool | Chat Completions | Repeated tool_calls indexing |
| 16 | Verify all tools reach model | Chat Completions | Tool definitions not truncated |
| 17 | Verify all tools reach model | Messages API | Tool definitions not truncated |
| 18 | Regression (no tools) | Chat Completions | Non-tool requests unaffected |
| 19 | Regression (no tools) | Messages API | Non-tool requests unaffected |

## Scale Down

To release GPUs while preserving the LLMInferenceService CR:

```bash
kubectl patch llminferenceservice <name> -n ${NAMESPACE} --type merge \
  -p '{"spec":{"replicas":0,"prefill":{"replicas":0}}}'
```

Scale back up:

```bash
kubectl patch llminferenceservice <name> -n ${NAMESPACE} --type merge \
  -p '{"spec":{"replicas":1,"prefill":{"replicas":1}}}'
```

## Cleanup

```bash
kubectl delete llminferenceservice --all -n ${NAMESPACE}
kubectl delete namespace ${NAMESPACE}
```
