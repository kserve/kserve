# Speculative Decoding Samples

These samples demonstrate how to configure speculative decoding with the `LLMInferenceService` CRD.
Speculative decoding uses a fast draft mechanism to propose candidate tokens that the target model
verifies in parallel, reducing the number of sequential decode steps and improving throughput.

## How It Works

Add a `spec.speculator` block to your `LLMInferenceService`. The `config` map is passed directly
to vLLM's `--speculative-config` JSON. For methods that require a separate model (Eagle3, draft-target,
Medusa), include `model.uri` and the controller will automatically download the speculator model via a
the shared `storage-initializer` init container.

See the [vLLM speculative decoding documentation](https://docs.vllm.ai/en/latest/features/speculative_decoding/#-speculative-config-schema)
for the full list of supported config keys.

## Samples

| File | Method | Target Model | Speculator/Draft | GPU |
|------|--------|-------------|------------------|-----|
| `llm-inference-service-H100-eagle3.yaml` | Eagle3 | Qwen3-32B FP8 | Eagle3 head | H100 (TP2, PD disagg) |
| `llm-inference-service-A100-draft-target.yaml` | draft_model | Llama-3.1-8B | Llama-3.2-1B | A100 |
| `llm-inference-service-L40S-draft-target.yaml` | draft_model | Llama-3.1-8B | Llama-3.2-1B | L40S |
| `llm-inference-service-A10-ngram.yaml` | N-gram | Gemma 3 4B | None (prompt lookup) | A10G |
| `llm-inference-service-H100-mtp.yaml` | MTP | Qwen3-Next-80B | None (built-in heads) | H100 (TP4) |

## Speculative Decoding Methods

- **Eagle3**: Purpose-trained speculator heads. Highest acceptance rates. Requires a per-model speculator.
- **Draft-target**: Uses a smaller model from the same family. No special training needed.
- **N-gram**: Proposes tokens by matching n-gram patterns from the input prompt. Zero overhead.
- **MTP (Multi-Token Prediction)**: Uses the target model's own built-in MTP heads. No separate model needed.
- **Medusa**: Multi-head decoder with tree-based attention. Requires a per-model Medusa head.

## vLLM V1 Engine

All samples set `VLLM_USE_V1=1` because vLLM's V1 engine is required for speculative decoding
with `--speculative-config`. The V0 engine does not support the `--speculative-config` flag.

## HuggingFace Token

For models hosted on HuggingFace that require authentication, create a Kubernetes secret
and reference it with the `serving.kserve.io/secretName` annotation:

```bash
kubectl create secret generic hf-secret --from-literal=HF_TOKEN=<your-token>
```
