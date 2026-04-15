# 02 - Dynamic LoRA Adapter Lifecycle

This directory demonstrates the core value proposition of LoRA serving:
**decoupling the adapter lifecycle from the model server lifecycle.**

LoRA adapters are loaded and unloaded at runtime via vLLM's management API --
no pod restarts, no CR updates, no downtime.

## Architecture

```
                    ┌──────────────────────┐
 External Clients ──┤  Gateway / HTTPRoute ├── /v1/chat/completions ──┐
                    └──────────────────────┘                          │
                                                                     ▼
                                                          ┌─────────────────┐
                    ┌──────────────────────┐              │ LLMInference-   │
 In-Cluster Jobs  ──┤  Direct pod access   ├── /v1/load_  │ Service         │
 (lora-manager SA)  │  (no Gateway)        │   lora_      │ (qwen2-lora)    │
                    └──────────────────────┘   adapter     │                 │
                                                          │ vLLM + LoRA     │
                              ▲                           │ --enable-lora   │
                              │                           └─────────────────┘
                    ┌─────────┴────────────┐
                    │  NetworkPolicy +     │
                    │  RBAC restrict who   │
                    │  can manage adapters │
                    └──────────────────────┘
```

**External clients** can only reach inference endpoints (`/v1/chat/completions`,
`/v1/completions`, `/v1/models`) via the Gateway.

**In-cluster workloads** running as the `lora-manager` ServiceAccount can call
vLLM's adapter management endpoints directly on the pod IPs.

## What's included

| File | Purpose |
|------|---------|
| `llm-inference-service-lora.yaml` | Base model with `--enable-lora` but no static adapters |
| `httproute.yaml` | Exposes only inference endpoints via Gateway |
| `network-policy.yaml` | Allows in-cluster traffic to vLLM pods |
| `rbac.yaml` | ServiceAccount, Role, and RoleBinding for adapter management |
| `lora-load-job.yaml` | Example Job to load a LoRA adapter on all replicas |
| `lora-unload-job.yaml` | Example Job to unload a LoRA adapter from all replicas |
| `kustomization.yaml` | Deploys the core infrastructure |

## Deployment

### 1. Deploy the base infrastructure

```bash
# Edit kustomization.yaml to set your namespace
# Edit httproute.yaml to set your gateway name and namespace

kubectl apply -k .
```

### 2. Wait for the model server to be ready

```bash
kubectl wait --for=condition=Ready llminferenceservice/qwen2-lora --timeout=600s
```

### 3. Load a LoRA adapter dynamically

```bash
kubectl apply -f lora-load-job.yaml
kubectl wait --for=condition=Complete job/lora-load-k8s-adapter --timeout=120s
kubectl logs job/lora-load-k8s-adapter
```

### 4. Verify the adapter is available

```bash
# List loaded models (should include both base model and k8s-lora)
curl -k https://<route-url>/v1/models | jq '.data[].id'
```

### 5. Run inference with the adapter

```bash
curl -k https://<route-url>/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "k8s-lora",
    "messages": [{"role": "user", "content": "How do I create a Kubernetes deployment?"}]
  }'
```

### 6. Unload the adapter (no restart needed)

```bash
kubectl apply -f lora-unload-job.yaml
kubectl wait --for=condition=Complete job/lora-unload-k8s-adapter --timeout=120s
```

## Security model

The security boundary is enforced at two levels:

1. **Gateway/HTTPRoute** -- External traffic is routed only to inference endpoints.
   The LoRA management endpoints (`/v1/load_lora_adapter`, `/v1/unload_lora_adapter`)
   are never exposed through the Gateway.

2. **RBAC** -- The `lora-manager` ServiceAccount has the minimum permissions needed
   to discover vLLM pods and manage adapters. Cluster operators control who can
   create Jobs using this ServiceAccount via standard Kubernetes RBAC.

## Adapter lifecycle operations

### Load a new adapter
The `lora-load-job.yaml` discovers all vLLM pods and calls `POST /v1/load_lora_adapter`
on each one. Edit the Job to change:
- `ADAPTER_NAME` -- the name clients use in the `model` field
- `ADAPTER_SOURCE` -- HuggingFace repo ID (vLLM downloads it)

### Unload an adapter
The `lora-unload-job.yaml` calls `POST /v1/unload_lora_adapter` on all pods.
The adapter is removed from GPU memory immediately.

### Swap an adapter
Load a new adapter and unload the old one -- the base model continues serving
throughout. Zero downtime.

### Automate with CronJobs
For scheduled adapter updates, convert the Jobs to CronJobs:

```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: nightly-adapter-refresh
spec:
  schedule: "0 2 * * *"  # 2 AM daily
  jobTemplate:
    spec:
      # ... same as lora-load-job.yaml
```

## What's next

See `03_production_hardening/` for adding rate limiting, authentication,
and quota management per adapter via Red Hat Connectivity Link (Kuadrant).
