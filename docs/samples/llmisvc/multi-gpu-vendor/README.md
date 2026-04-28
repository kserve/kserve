# Multi-GPU-Vendor Deployment Example

Deploy the same model across NVIDIA and AMD GPUs using two LLMInferenceService instances that share a single
scheduler/EPP via workload label propagation.

## Problem

A cluster has both NVIDIA and AMD GPU nodes. A single LLMInferenceService can only target one GPU type because all
replicas share the same pod template. Without this pattern, one GPU vendor's capacity sits idle.

## Solution

1. **NVIDIA instance** (`qwen2-7b-instruct-nvidia`) — creates the scheduler/EPP, InferencePool, HTTPRoute, and Gateway.
   The InferencePool uses the default selector (`app.kubernetes.io/name: qwen2-7b-instruct-nvidia`,
   `app.kubernetes.io/part-of: llminferenceservice`, `kserve.io/component: workload`).
2. **AMD instance** (`qwen2-7b-instruct-amd`) — has **no router** (no scheduler, no route, no gateway). Its pods carry
   the NVIDIA instance's workload labels via `spec.labels`, so the EPP discovers and routes traffic to them.

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
                      app.kubernetes.io/name: qwen2-7b-instruct-nvidia
                      app.kubernetes.io/part-of: llminferenceservice
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

The controller sets the InferencePool selector using the NVIDIA instance's default workload labels
(`app.kubernetes.io/name`, `app.kubernetes.io/part-of`, `kserve.io/component`). To make the AMD pods
discoverable by the same InferencePool, the AMD instance overrides these labels via `spec.labels`:

```yaml
# AMD instance — no router, labels match the NVIDIA InferencePool selector
spec:
  labels:
    app.kubernetes.io/name: qwen2-7b-instruct-nvidia
    app.kubernetes.io/part-of: llminferenceservice
    kserve.io/component: workload
  # no router section
```

Because `spec.labels` is propagated to the pod template **after** the controller sets its own default labels,
the AMD pods' `app.kubernetes.io/name` is overwritten from `qwen2-7b-instruct-amd` to
`qwen2-7b-instruct-nvidia`, making them match the NVIDIA InferencePool's selector.

The two Deployments do not conflict despite sharing label values because Kubernetes injects a unique
`pod-template-hash` label into each Deployment's ReplicaSet selector, keeping pod ownership isolated.

## Prerequisites

- Kubernetes cluster with both NVIDIA and AMD GPU nodes
- Nodes labeled appropriately (e.g., `nvidia.com/gpu.present: "true"`, `amd.com/gpu.present: "true"`)
- Model weights accessible via HuggingFace

## Deployment

```bash
# 1. Deploy the NVIDIA instance (creates InferencePool, EPP, HTTPRoute, Gateway)
kubectl apply -f llm-inference-service-qwen2-7b-nvidia-with-scheduler.yaml

# 2. Deploy the AMD instance (pods join the existing InferencePool via shared labels)
kubectl apply -f llm-inference-service-qwen2-7b-amd-no-scheduler.yaml
```

## Configuration Summary

| Feature         | NVIDIA Instance                   | AMD Instance                           |
|-----------------|-----------------------------------|----------------------------------------|
| Replicas        | 3                                 | 2                                      |
| GPU             | 1x NVIDIA per replica             | 1x AMD per replica                     |
| Scheduler / EPP | Yes (creates InferencePool + EPP) | No                                     |
| Route / Gateway | Yes                               | No                                     |
| Labels override | None (uses defaults)              | Overrides `app.kubernetes.io/name` etc. |

## Verification

```bash
# Check both services
kubectl get llminferenceservice

# Verify pods are on the correct GPU nodes
kubectl get pods -o wide -l app.kubernetes.io/name=qwen2-7b-instruct-nvidia

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

The EPP automatically discovers new pods as they match the workload labels.