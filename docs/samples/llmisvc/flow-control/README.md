# Flow Control

Example configuration enabling the experimental FlowControl feature for priority-based queuing, fairness, and saturation management.

## Overview

The Flow Control layer acts as a gatekeeper between routing and scheduling. It decides *if* and *when* a request should proceed to be scheduled onto a backend. When the system is saturated, requests are queued and dispatched through a **3-tier dispatch hierarchy**:

1. **Priority (Band Selection)** — High-priority traffic is served first
2. **Fairness (Flow Selection)** — Requests are distributed fairly between tenants/models within a priority band (e.g., round-robin)
3. **Ordering (Request Selection)** — Requests within a flow are ordered (e.g., FCFS, SLO deadline)

## Prerequisites

- Kubernetes cluster with GPU nodes
- Model weights accessible via HuggingFace or PVC

## Example

### [`llm-inference-service-flow-control.yaml`](llm-inference-service-flow-control.yaml)

Deploys Qwen2.5-7B-Instruct with 2 replicas and FlowControl enabled with three priority bands.

**Priority Bands:**

| Priority | Ordering Policy | Fairness Policy | Use Case |
|----------|----------------|-----------------|----------|
| 1 | `slo-deadline-ordering-policy` | `round-robin-fairness-policy` | Latency-sensitive requests with SLO targets |
| 0 | FCFS (default) | `round-robin-fairness-policy` | Standard requests (default when no `InferenceObjective` is set) |
| -1 | FCFS (default) | `round-robin-fairness-policy` | Best-effort or background requests |

**Saturation Thresholds:**

| Parameter | Value | Description |
|-----------|-------|-------------|
| `queueDepthThreshold` | 3 | Backend waiting queue size above which a pod is considered to have insufficient capacity |
| `kvCacheUtilThreshold` | 0.8 | KV cache utilization above which a pod is considered to have insufficient capacity |

When saturation reaches 1.0, head-of-line blocking is enforced and requests are queued until capacity is available.

## Request Headers

Clients set priority, tenant identity, and SLO targets via headers:

| Header | Purpose | Default |
|--------|---------|---------|
| `x-gateway-inference-objective` | References an `InferenceObjective` CR; its `.spec.priority` determines the request's priority band | Priority 0 |
| `x-gateway-inference-fairness-id` | Identifies the tenant/flow for fairness within a priority band | `default-flow` |
| `x-slo-ttft-ms` | Time-to-first-token target in milliseconds; `slo-deadline-ordering-policy` computes deadline as `received_time + value` | No deadline (ordered last) |

```bash
curl -H "x-gateway-inference-objective: premium-slo" \
     -H "x-gateway-inference-fairness-id: tenant-a" \
     -H "x-slo-ttft-ms: 500" \
     -H "Content-Type: application/json" \
     -d '{"model": "Qwen/Qwen2.5-7B-Instruct", "prompt": "...", "max_tokens": 100}' \
     https://<route>/v1/completions
```

Each unique `(fairnessID, priority)` pair is a separate flow. Requests arriving with a priority that does not match any configured band are automatically handled — a new band is dynamically provisioned using the `defaultPriorityBand` template and garbage-collected after 10 minutes of inactivity.

## Deployment

```bash
kubectl apply -f llm-inference-service-flow-control.yaml
```

## Tuning

### Saturation and vLLM

Per-pod saturation is `Max(WaitingQueueSize / queueDepthThreshold, KVCacheUsage / kvCacheUtilThreshold)`, averaged across pods. The EPP starts queuing when this reaches 1.0. Both inputs come from vLLM metrics (`vllm:num_requests_waiting` and `vllm:kv_cache_usage_perc`), so three vLLM flags directly affect when saturation triggers:

**`--max-num-seqs`** — Caps concurrent sequences per instance. Requests beyond this limit, along with requests deferred for other reasons (e.g., LoRA budget, KV transfer), enter the engine's waiting queue. The `queueDepthThreshold` monitors this queue via `vllm:num_requests_waiting`. A lower `queueDepthThreshold` relative to `--max-num-seqs` causes the EPP to start queuing sooner, reducing tail latency at the cost of throughput. A higher value lets the engine absorb more backlog before the EPP intervenes.

**`--gpu-memory-utilization`** — Fraction of GPU memory available to the model executor, from which the engine derives the KV cache block budget. A lower value reduces total KV cache capacity, causing `kvCacheUtilThreshold` to trigger sooner under the same request load.

**`--max-model-len`** — Caps maximum sequence length (prompt + output). A request's KV cache consumption is proportional to its actual sequence length. Higher `--max-model-len` allows longer sequences, which consume more KV cache per request, so fewer can run concurrently before `kvCacheUtilThreshold` triggers.

### Other Parameters

- **`maxBytes`** — Global capacity limit; new requests are rejected if exceeded
- **`defaultRequestTTL`** — Fallback timeout for requests without a deadline
- **Omit `priorityBands`** to let the system dynamically provision bands with FCFS ordering, `global-strict-fairness-policy` (no tenant isolation — a noisy neighbor can starve other flows), and 1GB capacity. Use explicit bands with `round-robin-fairness-policy` for multi-tenant fairness.
