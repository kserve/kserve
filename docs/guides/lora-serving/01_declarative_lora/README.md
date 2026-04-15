# 01 - Declarative LoRA Adapter Support

This directory shows LoRA adapter serving using the declarative `model.lora.adapters`
API introduced in [kserve/kserve#5317](https://github.com/kserve/kserve/pull/5317).

## What changed from 00

Instead of manually wiring vLLM arguments via `VLLM_ADDITIONAL_ARGS`, adapters are now
declared as first-class fields on the LLMInferenceService:

```yaml
spec:
  model:
    uri: hf://Qwen/Qwen2.5-7B-Instruct
    name: Qwen/Qwen2.5-7B-Instruct
    lora:
      adapters:
        - name: k8s-lora
          uri: hf://cimendev/kubernetes-qa-qwen2.5-7b-lora
        - name: finance-lora
          uri: hf://Max1690/qwen2.5-7b-finance-lora
```

The controller automatically:
1. Downloads adapter weights via the storage initializer (for `hf://` and `s3://` URIs)
2. Mounts adapter weights at `/mnt/lora/<adapter-name>`
3. Injects vLLM flags (`--enable-lora`, `--max-lora-rank`, `--lora-modules`, etc.)

No `VLLM_ADDITIONAL_ARGS`, no `HF_HUB_OFFLINE=false` — the controller handles it all.

## Supported URI schemes

| Scheme   | Description                      | Example                               |
|----------|----------------------------------|---------------------------------------|
| `hf://`  | HuggingFace Hub                  | `hf://my-org/my-lora-adapter`         |
| `s3://`  | S3-compatible object storage     | `s3://my-bucket/adapters/lora-v1`     |
| `pvc://` | Kubernetes PersistentVolumeClaim | `pvc://my-pvc/path/to/adapter`        |

## Inference

Clients select which adapter to use via the `model` field in the OpenAI-compatible API:

```bash
# Use the k8s-lora adapter
curl -k https://<route-url>/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "k8s-lora",
    "messages": [{"role": "user", "content": "How do I create a Kubernetes deployment?"}]
  }'

# Use the base model directly (no adapter)
curl -k https://<route-url>/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "Qwen/Qwen2.5-7B-Instruct",
    "messages": [{"role": "user", "content": "What is Kubernetes?"}]
  }'
```

## Deployment

```bash
# Edit kustomization.yaml to set your namespace
# Edit httproute.yaml to set your gateway name and namespace

kubectl apply -k .
```

## Remaining limitation

Adapters are still **statically declared** at deploy time. Adding or removing an adapter
requires updating the LLMInferenceService CR, which triggers a new rollout.

See `02_dynamic_lora_lifecycle/` for dynamic adapter loading and unloading without
restarting the model server.
