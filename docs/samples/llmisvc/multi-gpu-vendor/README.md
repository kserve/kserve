# Multi-GPU-Vendor Deployment Example

Deploy the same model across NVIDIA and AMD GPUs using two LLMInferenceService instances that share a single
scheduler/EPP via a custom InferencePool selector.

## Problem

A cluster has both NVIDIA and AMD GPU nodes. A single LLMInferenceService can only target one GPU type because all
replicas share the same pod template. Without this pattern, one GPU vendor's capacity sits idle.

## Solution

1. **NVIDIA instance** (`qwen2-7b-instruct-nvidia`) — creates the scheduler/EPP, InferencePool, HTTPRoute, and Gateway.
   The InferencePool uses a custom selector (`llm-pool: qwen2-7b`) instead of the default name-based selector.
2. **AMD instance** (`qwen2-7b-instruct-amd`) — has **no router** (no scheduler, no route, no gateway). Its pods carry
   the same `llm-pool: qwen2-7b` label, so the EPP from the NVIDIA instance discovers and routes traffic to them.

## Architecture

```
                         ┌──────────────┐
                         │   Gateway    │
                         └──────┬───────┘
                                │
                         ┌──────┴───────┐
                         │  HTTPRoute   │
                         └──────┬───────┘
                                │
                    ┌───────────┴───────────┐
                    │   Scheduler / EPP     │
                    │  (from NVIDIA isvc)   │
                    └───────────┬───────────┘
                                │
                    InferencePool selector:
                      llm-pool: qwen2-7b
                      kserve.io/component: workload
                                │
              ┌─────────────────┼─────────────────┐
              │                                   │
   ┌──────────┴──────────┐             ┌──────────┴──────────┐
   │  NVIDIA vLLM Pods   │             │   AMD vLLM Pods     │
   │  (3 replicas)       │             │   (2 replicas)      │
   │  nvidia.com/gpu: 1  │             │  amd.com/gpu: 1     │
   └─────────────────────┘             └─────────────────────┘
```

## How It Works

By default, the controller sets the InferencePool selector to match on `app.kubernetes.io/name: <service-name>`,
which only selects pods from that single LLMInferenceService. To pool pods from multiple instances together, the
NVIDIA instance overrides the selector with a shared custom label:

```yaml
# NVIDIA instance — custom pool selector
spec:
  labels:
    llm-pool: qwen2-7b          # added to all workload pods
  router:
    scheduler:
      pool:
        spec:
          selector:
            matchLabels:
              llm-pool: qwen2-7b           # shared across instances
              kserve.io/component: workload # only select workload pods
```

The AMD instance carries the same label but has no router:

```yaml
# AMD instance — no router, shared label
spec:
  labels:
    llm-pool: qwen2-7b          # matches the InferencePool selector
  # no router section
```

## Prerequisites

- Kubernetes cluster with both NVIDIA and AMD GPU nodes
- Nodes labeled appropriately (e.g., `nvidia.com/gpu.present: "true"`, `amd.com/gpu.present: "true"`)
- Model weights accessible via HuggingFace

## Deployment

```bash
# 1. Deploy the NVIDIA instance (creates InferencePool, EPP, HTTPRoute, Gateway)
kubectl apply -f llm-inference-service-qwen2-7b-nvidia-with-scheduler.yaml

# 2. Deploy the AMD instance (pods join the existing InferencePool via shared label)
kubectl apply -f llm-inference-service-qwen2-7b-amd-no-scheduler.yaml
```

## Configuration Summary

| Feature         | NVIDIA Instance                   | AMD Instance         |
|-----------------|-----------------------------------|----------------------|
| Replicas        | 3                                 | 2                    |
| GPU             | 1x NVIDIA per replica             | 1x AMD per replica   |
| Scheduler / EPP | Yes (creates InferencePool + EPP) | No                   |
| Route / Gateway | Yes                               | No                   |
| Shared label    | `llm-pool: qwen2-7b`              | `llm-pool: qwen2-7b` |

## Verification

```bash
# Check both services
kubectl get llminferenceservice

# Verify pods are on the correct GPU nodes
kubectl get pods -o wide -l llm-pool=qwen2-7b

# Confirm the InferencePool selects pods from both instances
kubectl get inferencepool -o yaml

# Check scheduler logs for routing across all replicas
kubectl logs -l app.kubernetes.io/component=llminferenceservice-scheduler -f

# Send a test request
curl -k https://<route-url>/v1/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "Qwen/Qwen2.5-7B-Instruct",
    "prompt": "What is Kubernetes?",
    "max_tokens": 100
  }'
```

## Scaling

Each instance can be scaled independently:

```bash
# Scale NVIDIA replicas
kubectl patch llmisvc qwen2-7b-instruct-nvidia --type merge -p '{"spec":{"replicas":5}}'

# Scale AMD replicas
kubectl patch llmisvc qwen2-7b-instruct-amd --type merge -p '{"spec":{"replicas":4}}'
```

The EPP automatically discovers new pods as they match the shared `llm-pool: qwen2-7b` label.