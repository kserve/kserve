# 01 - Dynamic LoRA Serving

This directory demonstrates **decoupling the adapter lifecycle from the model server
lifecycle** using only the capabilities available today (v1alpha1, `VLLM_ADDITIONAL_ARGS`).

LoRA adapters are loaded and unloaded at runtime via vLLM's management API —
no pod restarts, no CR updates, no downtime.

## How it differs from 00_static_lora

In `00_static_lora`, adapters are baked into the deployment via `--lora-modules`.
Here, the LLMInferenceService starts with `--enable-lora` but **no static adapters**.
In-cluster Jobs call vLLM's `/v1/load_lora_adapter` and `/v1/unload_lora_adapter`
endpoints to manage adapters at runtime.

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

1. **Gateway/HTTPRoute** — External traffic is routed only to inference endpoints.
   The LoRA management endpoints (`/v1/load_lora_adapter`, `/v1/unload_lora_adapter`)
   are never exposed through the Gateway.

2. **RBAC** — The `lora-manager` ServiceAccount has the minimum permissions needed
   to discover vLLM pods and manage adapters. Cluster operators control who can
   create Jobs using this ServiceAccount via standard Kubernetes RBAC.

## Limitations

- **Still uses `VLLM_ADDITIONAL_ARGS`.** The controller has no awareness of LoRA
  configuration — `--enable-lora` and related flags are passed as raw vLLM args.
- **No storage orchestration.** vLLM downloads adapters from HuggingFace at load
  time (`HF_HUB_OFFLINE=false`). No pre-download via the storage initializer.
- **Manual pod discovery.** Jobs must discover vLLM pod IPs via the Kubernetes API
  and call each pod individually.
