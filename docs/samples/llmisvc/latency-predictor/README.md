# Latency Predictor Scheduling

This directory contains an example configuration demonstrating latency-aware scheduling using the `predicted-latency-producer` scheduler plugin.

## Overview

The latency predictor feature enables the router to estimate per-endpoint request latency (TTFT and TPOT) using continuously retrained XGBoost models. When the controller detects the `predicted-latency-producer` plugin in `spec.router.scheduler.config.inline`, it automatically injects training and prediction server sidecar containers into the router pod.

For the upstream llm-d guide on this feature, see [predicted-latency-routing](https://github.com/llm-d/llm-d/tree/main/guides/predicted-latency-routing).

## Prerequisites

- Kubernetes cluster with GPU nodes
- Model weights accessible via HuggingFace or PVC

## Example

### Latency-Aware Deployment ([`llm-inference-service-latency-predictor.yaml`](llm-inference-service-latency-predictor.yaml))

**Configuration:**

- Model: Qwen2.5-7B-Instruct
- Replicas: 3
- GPU per replica: 1
- Router: Latency predictor with latency scoring

**Scheduler Plugins:**

| Plugin | Purpose |
|---|---|
| `approx-prefix-cache-producer` | Computes approximate prefix cache hit ratios for each endpoint |
| `predicted-latency-producer` | Produces predicted TTFT/TPOT for each endpoint |
| `prefix-cache-affinity-filter` | Filters endpoints based on prefix cache affinity threshold |
| `latency-scorer` | Scores endpoints using the predicted latency |
| `weighted-random-picker` | Selects an endpoint using weighted random selection |

**Deployment:**

```bash
kubectl apply -f llm-inference-service-latency-predictor.yaml
```

## Important Notes

- The `predicted-latency-producer` plugin must be specified in `config.inline`, **not** in `config.ref` (ConfigMap-based config). Sidecar injection detection runs before ConfigMap resolution.
- Latency predictions improve over time as the training server collects more samples. Initial requests fall back to composite heuristic scoring until enough data is available.
- For SLO-aware scheduling with `x-llm-d-slo-ttft-ms` / `x-llm-d-slo-tpot-ms` headers, set `streamingMode: true` on the `predicted-latency-producer` plugin and add the `slo-headroom-tier-filter` and `latency-slo-admitter` plugins. See the [upstream SLO-aware configuration](https://github.com/llm-d/llm-d/tree/main/guides/predicted-latency-routing) for details.

## Monitoring

```bash
# Check that the router pod has the training and prediction sidecars
kubectl get pods -l app.kubernetes.io/component=llminferenceservice-router-scheduler -o jsonpath='{.items[*].spec.containers[*].name}'
# Expected: main tokenizer training-server prediction-server

# View training server logs
kubectl logs -l app.kubernetes.io/component=llminferenceservice-router-scheduler -c training-server

# View prediction server logs
kubectl logs -l app.kubernetes.io/component=llminferenceservice-router-scheduler -c prediction-server
```
