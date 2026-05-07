# Preload LoRA Adapters with LLMInferenceService

Demonstrates declarative multi-LoRA adapter management via `spec.model.lora.adapters`.

## What this sample does

The KServe controller reads `spec.model.lora.adapters` and:

1. Renders `--enable-lora`, `--max-loras`, `--max-lora-rank`, and `--lora-modules`
   into the vLLM startup command automatically.
2. Adds one `adapter-fetch-<name>` init container per adapter that downloads
   weights into `/mnt/loras/<name>` using the KServe storage initializer.
3. Creates one `InferenceObjective` (GIE `apix/v1alpha2`) per adapter, owned by
   the `LLMInferenceService`, for EPP `lora-affinity-scorer` routing.

After applying, clients route to an adapter by passing `"model": "<adapter-name>"`
in OpenAI-compatible requests (`/v1/chat/completions`, `/v1/completions`).

## Prerequisites

- KServe LLMInferenceService controller installed.
- [gateway-api-inference-extension (GIE)](https://github.com/kubernetes-sigs/gateway-api-inference-extension)
  installed for `InferenceObjective`-based routing. Without GIE, base-model
  serving still works; adapter routing in the data plane requires GIE EPP.
- A node with GPU memory sufficient for the base model plus concurrent adapters.
  For CPU testing, replace the image with a vLLM CPU build.
- HF_TOKEN is not required for `hf://facebook/opt-125m` (public). Set it in
  the pod spec if you use a gated model.

## Apply

Edit `inference_lora_preload.yaml` to set your adapter URIs and image, then:

```bash
kubectl apply -f inference_lora_preload.yaml
```

## Verify

Check controller-emitted objects:

```bash
# Init containers in the Deployment spec
kubectl get deployment opt-125m-with-lora-main \
  -o jsonpath='{.spec.template.spec.initContainers[*].name}'

# InferenceObjective objects
kubectl get inferenceobjectives -l app.kubernetes.io/name=opt-125m-with-lora

# Detailed view of one IO
kubectl describe inferenceobjectiveobjective opt-125m-with-lora-adapter-customer-a
```

Send a request to the adapter once the service is Ready:

```bash
SERVICE_URL=$(kubectl get llminferenceservice opt-125m-with-lora \
  -o jsonpath='{.status.addresses[0].url}')

curl -s "${SERVICE_URL}/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -d '{"model": "customer-a", "messages": [{"role": "user", "content": "Hello"}], "max_tokens": 50}'
```

## Tracking

Tracking issue: https://github.com/kserve/kserve/issues/3750

Dynamic (hot-swap) adapter loading and per-adapter status conditions are planned
for a follow-up PR.
