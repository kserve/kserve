# Single-Node GPU Deployment Examples

This directory contains example configurations for deploying LLM inference services on single-node GPU setups, ranging
from basic load balancing to advanced prefill-decode separation with KV cache transfer.

## Overview

These examples demonstrate various deployment patterns for single-GPU-per-replica workloads, suitable for development,
testing, or production deployments of smaller models (e.g., 7B parameter models).

## Prerequisites

- Kubernetes cluster with GPU nodes
- Model weights accessible via HuggingFace or PVC
- For prefill-decode examples: RoCE network configuration and RDMA support

## Examples

### 1. Basic Deployment with Default Scheduler ([

`llm-inference-service-qwen2-7b-gpu.yaml`](llm-inference-service-qwen2-7b-gpu.yaml))

Simplest deployment with default scheduler configuration.

**Configuration:**

- Model: Qwen2.5-7B-Instruct
- Replicas: 3
- GPU per replica: 1
- Scheduler: Default

**Use Case:**

- Development and testing
- Simple load balancing across replicas
- No special routing logic needed

**Deployment:**

```bash
kubectl apply -f llm-inference-service-qwen2-7b-gpu.yaml
```

### 2. Deployment Without Scheduler ([

`llm-inference-service-qwen2-7b-gpu-no-scheduler.yaml`](llm-inference-service-qwen2-7b-gpu-no-scheduler.yaml))

Direct routing without scheduler component.

**Configuration:**

- Model: Qwen2.5-7B-Instruct
- Replicas: 3
- GPU per replica: 1
- Scheduler: None (direct Kubernetes service load balancing)

**Use Case:**

- Minimal overhead deployment
- When advanced scheduling features are not needed
- Direct load balancing via Kubernetes service

**Key Difference:**

- No scheduler pod deployed
- Routing handled by Kubernetes service (kube-proxy)
- Lower resource overhead
- No support for advanced routing (prefix cache, prefill-decode separation, etc.)

**Deployment:**

```bash
kubectl apply -f llm-inference-service-qwen2-7b-gpu-no-scheduler.yaml
```

### 3. Prefill-Decode Separation ([

`llm-inference-service-pd-qwen2-7b-gpu.yaml`](llm-inference-service-pd-qwen2-7b-gpu.yaml))

Advanced configuration separating prefill and decode workloads with KV cache transfer over RDMA.

**Configuration:**

- Model: Qwen2.5-7B-Instruct
- Main/Decode pool: 1 replica
- Prefill pool: 2 replicas
- GPU per replica: 1
- RDMA: RoCE network required
- KV transfer: Enabled via NixlConnector

**Key Features:**

- **Prefill-Decode Separation**: Requests are routed to dedicated prefill or decode pools based on request type
- **KV Cache Transfer**: KV cache blocks transferred from prefill to decode pods via RDMA
- **Multiple Prefill Replicas**: 2 prefill pods for higher concurrent prefill throughput
- **Threshold-Based Routing**: `threshold: 0` means always use prefill-decode separation
- **Prefix Cache Scoring**: Routes to endpoints with matching KV cache (weight: 2.0)
- **Load-Aware Scheduling**: Balances load across endpoints (weight: 1.0)
- **UCX Transport Configuration**: `UCX_TLS` explicitly configured for optimal RDMA performance
- **Pod Anti-Affinity** (commented): Optional setting to force KV transfer over RDMA instead of NVLink
- **Dynamic Pool Naming**: Uses Go templates for automatic pool name generation

**Use Case:**

- High-concurrency production workloads
- When prefill is the bottleneck
- Clusters with specific RDMA/UCX requirements

**Pod Anti-Affinity Note:**

The commented-out affinity rules can be enabled to ensure prefill and decode pods are placed on different nodes, forcing
KV cache transfer to use the RDMA network instead of NVLink (if pods were on the same node).

**Deployment:**

```bash
# Ensure RoCE network is configured
kubectl apply -f network-roce-p2.yaml

# Deploy the service
kubectl apply -f llm-inference-service-pd-qwen2-7b-gpu.yaml
```

## Scheduler Configuration Comparison

| Feature                   | No Scheduler       | Default Scheduler    | Prefill-Decode           |
|---------------------------|--------------------|----------------------|--------------------------|
| Routing Logic             | Kubernetes Service | Basic load balancing | Advanced (PD separation) |
| Prefix Cache Routing      | ❌                  | ✅                    | ✅                        |
| Prefill-Decode Separation | ❌                  | ❌                    | ✅                        |
| KV Cache Transfer         | ❌                  | ❌                    | ✅                        |
| Resource Overhead         | Lowest             | Low                  | Medium                   |
| Use Case                  | Simple/Dev         | Production (basic)   | Production (advanced)    |

## Resource Requirements

All examples use the same per-replica resources:

- 1 NVIDIA GPU
- 4 CPU cores (limit), 2 cores (request)
- 32Gi memory (limit), 16Gi memory (request)

For prefill-decode examples with RDMA:

- 1 `rdma/roce_gdr` resource

## Prefill-Decode Separation Explained

### What is Prefill-Decode Separation?

LLM inference consists of two phases:

1. **Prefill**: Process the entire input prompt to generate the KV cache
    - Compute-intensive
    - Parallelizable across tokens
    - Happens once per request

2. **Decode**: Generate output tokens one at a time using the KV cache
    - Memory-intensive
    - Sequential (cannot parallelize across tokens)
    - Happens multiple times per request (once per output token)

### Why Separate Them?

- **Resource Optimization**: Prefill pods can have different resource configurations than decode pods
- **Better Throughput**: Dedicated prefill pods can handle multiple concurrent prefill requests
- **Reduced Latency**: Decode pods don't compete for resources with prefill operations
- **KV Cache Transfer**: Prefill generates the KV cache, which is transferred to decode pods via RDMA

### How the Scheduler Routes Requests

1. **New Request (no KV cache)** → Routed to **Prefill Pool**
2. **Continuation Request (has KV cache)** → Routed to **Decode Pool**
3. **KV Cache Transfer**: After prefill completes, KV cache is transferred from prefill pod to decode pod via RDMA

### Threshold Parameter

The `threshold: 0` parameter in the scheduler config determines when to use prefill-decode separation:

- `threshold: 0` → Always separate (all requests go through prefill-decode)
- `threshold: N` → Only separate if estimated tokens > N

## Monitoring

### Check Service Status

```bash
# List all LLMInferenceServices
kubectl get llminferenceservice -owide

# Check pod status
kubectl get pods -l app.kubernetes.io/component=llminferenceservice-workload

# For prefill-decode examples, check both pools
kubectl get pods -l app.kubernetes.io/component=llminferenceservice-workload
kubectl get pods -l app.kubernetes.io/component=llminferenceservice-workload-prefill
```

### View Logs

```bash
# Scheduler logs (if using scheduler)
kubectl logs -l app.kubernetes.io/component=llminferenceservice-scheduler -f

# Inference pod logs (main/decode)
kubectl logs <pod-name> -c main

# Prefill pod logs
kubectl logs <prefill-pod-name> -c main
```

### Test Inference

```bash
# Get the service URL
kubectl get route -l serving.kserve.io/inferenceservice=<service-name>

# Send a test request
curl -k https://<route-url>/v1/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "Qwen/Qwen2.5-7B-Instruct",
    "prompt": "What is Kubernetes?",
    "max_tokens": 100
  }'
```

## Choosing the Right Example

- **Production (Simple)**: Use example #1 (default scheduler)
- **Development/Testing**: #2 (no scheduler)
- **Production (Advanced, RDMA available)**: Use example #3 (prefill-decode)

## Troubleshooting

### Pods Not Starting

- Check GPU availability: `kubectl describe node <node-name> | grep nvidia.com/gpu`
- Check resource quotas
- Review pod events: `kubectl describe pod <pod-name>`

### RDMA Issues (Prefill-Decode Examples)

- Verify RoCE network attachment: `kubectl get network-attachment-definitions`
- Check RDMA device availability: `kubectl describe node <node-name> | grep rdma/roce_gdr`
- Review UCX logs in pod output

### KV Cache Transfer Failures

- Ensure `VLLM_NIXL_SIDE_CHANNEL_HOST` is correctly set to pod IP
- Check RDMA connectivity between prefill and decode pods
- Verify `KSERVE_INFER_ROCE=true` is set
- Review scheduler logs for routing decisions

### Performance Issues

- Monitor GPU utilization: Use NVIDIA DCGM or similar tools
- Check request distribution across replicas
- Review scheduler metrics for routing effectiveness
- Consider increasing replica count for higher throughput