# 05 - Per-Adapter Observability

This directory provides monitoring and alerting for LoRA adapter serving,
giving platform teams visibility into how each adapter is performing
independently.

## What you get

### Grafana Dashboard

The `grafana-lora-dashboard.json` provides 7 panels:

| Panel | Metric | Why it matters |
|-------|--------|----------------|
| Request Rate by Adapter | `vllm:request_success_total` | Which adapters are getting traffic? |
| Request Latency (p50/p95/p99) | `vllm:e2e_request_latency_seconds` | Is one adapter slower than others? |
| Input Token Throughput | `vllm:prompt_tokens_total` | How much input are adapters processing? |
| Output Token Throughput | `vllm:generation_tokens_total` | How much output are adapters generating? |
| Time to First Token (p50/p95) | `vllm:time_to_first_token_seconds` | Interactive responsiveness per adapter |
| GPU/CPU KV Cache Utilization | `vllm:gpu_cache_usage_perc` | Memory pressure from loaded adapters |
| Running/Waiting Requests | `vllm:num_requests_running/waiting` | Is an adapter saturating the server? |

All per-adapter panels use the `model_name` label, which vLLM sets to the
adapter name for LoRA requests and to the base model name otherwise.

### Prometheus Alerts

The `prometheus-rules.yaml` defines three alerts:

| Alert | Condition | Severity |
|-------|-----------|----------|
| `LoRAAdapterHighLatency` | p99 latency > 30s for 5 min | warning |
| `LoRAAdapterHighErrorRate` | Error rate > 5% for 5 min | critical |
| `LoRAHighGPUMemory` | GPU cache usage > 80% for 10 min | warning |

## Prerequisites

- Prometheus Operator (or OpenShift built-in monitoring)
- Grafana (or OpenShift console dashboards)
- LLMInferenceService with LoRA adapters serving traffic

## Deployment

### ServiceMonitor + Alerts

```bash
# Edit kustomization.yaml to set your namespace
# Edit service-monitor.yaml namespaceSelector to match your deployment namespace

kubectl apply -k .
```

### Grafana Dashboard

Import `grafana-lora-dashboard.json` into Grafana:

```bash
# Via Grafana UI: Dashboards → Import → Upload JSON file

# Or via API:
curl -X POST http://grafana:3000/api/dashboards/db \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <api-key>" \
  -d "{\"dashboard\": $(cat grafana-lora-dashboard.json), \"overwrite\": true}"
```

## Key queries for ad-hoc investigation

### Which adapters are loaded right now?

```promql
count by(model_name) (vllm:num_requests_running >= 0)
```

### Adapter request share (% of total traffic)

```promql
sum by(model_name) (rate(vllm:request_success_total[5m]))
/
sum (rate(vllm:request_success_total[5m]))
```

### Token cost per adapter (useful for chargeback)

```promql
sum by(model_name) (increase(vllm:prompt_tokens_total[24h]) + increase(vllm:generation_tokens_total[24h]))
```

### Adapter latency comparison

```promql
histogram_quantile(0.95,
  sum(rate(vllm:e2e_request_latency_seconds_bucket[5m])) by (le, model_name)
)
```

## Integrating with OpenShift monitoring

On OpenShift, the ServiceMonitor is picked up automatically by the
built-in Prometheus. To view metrics:

1. Navigate to **Observe** → **Metrics** in the OpenShift console
2. Enter any of the PromQL queries above
3. For dashboards, use the OpenShift console's dashboard feature or
   deploy Grafana via the Grafana Operator
