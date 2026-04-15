# 00 - Existing LoRA Support

This directory shows LoRA adapter serving with the support available today, **before**
the declarative `model.lora.adapters` API lands in the controller.

## How it works

LoRA adapters are configured manually via vLLM CLI arguments passed through the
`VLLM_ADDITIONAL_ARGS` environment variable:

```yaml
env:
  - name: VLLM_ADDITIONAL_ARGS
    value: "--enable-lora --max-lora-rank=64 --max-loras=2 --max-cpu-loras=2 --lora-modules k8s-lora=cimendev/kubernetes-qa-qwen2.5-7b-lora finance-lora=Max1690/qwen2.5-7b-finance-lora"
```

vLLM downloads the adapters from HuggingFace at startup and loads them into GPU memory.
Clients select an adapter by setting the `model` field in their OpenAI-compatible request:

```bash
curl -k https://<route-url>/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "k8s-lora",
    "messages": [{"role": "user", "content": "How do I create a Kubernetes deployment?"}]
  }'
```

## Limitations

- **Adapter lifecycle is coupled to the model server lifecycle.** Adding, removing, or
  updating a LoRA adapter requires editing `VLLM_ADDITIONAL_ARGS` and re-rolling the
  deployment. This restarts vLLM and causes downtime.
- **No declarative API.** The controller has no awareness of which adapters are loaded;
  configuration is a raw string passed to vLLM.
- **No storage orchestration.** The controller does not download adapter weights via the
  storage initializer. vLLM must download them itself at startup (`HF_HUB_OFFLINE=false`).

## Deployment

```bash
# Edit kustomization.yaml to set your namespace
# Edit httproute.yaml to set your gateway name and namespace

kubectl apply -k .
```

## What's next

See `01_declarative_lora/` for the improved experience using the declarative
`model.lora.adapters` API, which decouples adapter management from vLLM arguments.
