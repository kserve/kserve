# Speculative Decoding with LLMInferenceService

Speculative decoding accelerates LLM inference by proposing multiple candidate tokens
cheaply and verifying them in a single target model forward pass. The output is
mathematically identical to standard decoding — it is a pure latency optimization.

LLMInferenceService provides first-class support via `spec.speculativeDecoding`:

```yaml
spec:
  speculativeDecoding:
    method: eagle3
    numSpeculativeTokens: 3
    speculator:
      model:
        uri: hf://RedHatAI/Qwen3-32B-speculator.eagle3
      tensorParallelSize: 1
      maxModelLen: 4096
    additionalConfig:       # escape hatch for runtime-specific params
      enforce_eager: "true"
```

The controller automatically handles speculator model download (via a second
storage-initializer init container), volume/mount setup, and `--speculative-config`
injection into vLLM.

## Supported methods

### Eagle3

Lightweight speculator head (~2B params) trained on the target model's hidden states.
Highest acceptance rates because the speculator directly sees the target's internal
representations.

- Requires a purpose-trained speculator head per target model
- Speculator is co-located on the same GPU(s) as the target
- `speculator` field required (model download needed)

```yaml
speculativeDecoding:
  method: eagle3
  numSpeculativeTokens: 3
  speculator:
    model:
      uri: hf://RedHatAI/Qwen3-32B-speculator.eagle3
    tensorParallelSize: 1
```

**Example:** [`llm-inference-service-qwen32b-eagle3.yaml`](llm-inference-service-qwen32b-eagle3.yaml)

### Draft-target

A smaller model from the same family generates draft tokens independently. The classic
approach — no special training needed, works with any compatible model pair that shares
the same vocabulary and tokenizer.

- Draft model is downloaded and co-located alongside the target
- `speculator` field required (model download needed)
- Lower acceptance rates than Eagle3 since the draft model doesn't see the target's
  internal state

```yaml
speculativeDecoding:
  method: draft_model
  numSpeculativeTokens: 5
  speculator:
    model:
      uri: hf://meta-llama/Llama-3.2-1B-Instruct
    tensorParallelSize: 1
    maxModelLen: 8192
```

**Examples:**
- [`llm-inference-service-llama8b-draft-target.yaml`](llm-inference-service-llama8b-draft-target.yaml) — A100 40GB
- [`llm-inference-service-llama8b-draft-target-l40s.yaml`](llm-inference-service-llama8b-draft-target-l40s.yaml) — L40S 48GB

### Medusa

Multi-head decoder trained on top of the target model. Similar to Eagle3 but uses
tree-based attention for candidate verification. Available Medusa heads for vLLM are
currently limited.

- Requires trained Medusa heads per target model
- `speculator` field required (model download needed)

```yaml
speculativeDecoding:
  method: medusa
  numSpeculativeTokens: 5
  speculator:
    model:
      uri: hf://abhigoyal/vllm-medusa-vicuna-7b-v1.3
    tensorParallelSize: 1
```

**Example:** [`llm-inference-service-vicuna7b-medusa.yaml`](llm-inference-service-vicuna7b-medusa.yaml)

### N-gram

Proposes draft tokens by matching n-gram patterns from the input prompt itself. Zero
overhead — no separate model, no extra memory. Effective for input-grounded tasks
like summarization, question-answering, and code completion where the output echoes
the input.

- No `speculator` field (no model download needed)
- `prompt_lookup_max` and `prompt_lookup_min` control n-gram matching window

```yaml
speculativeDecoding:
  method: ngram
  numSpeculativeTokens: 4
  additionalConfig:
    prompt_lookup_max: "5"
    prompt_lookup_min: "2"
```

**Example:** [`llm-inference-service-gemma3-ngram.yaml`](llm-inference-service-gemma3-ngram.yaml)

### MTP (Multi-Token Prediction)

Uses the target model's own built-in MTP heads to speculate. No separate model needed,
but the model must have been trained with MTP support.

- No `speculator` field (no model download needed)
- Only works with models that have native MTP heads

**Runtime-specific method variants:** Some model families require a specific MTP method
name to be passed to vLLM. The CRD `method: mtp` provides the generic abstraction, and
`additionalConfig` can refine it for the runtime:

| Model family | vLLM method | additionalConfig needed? |
|---|---|---|
| DeepSeek-V3, Qwen3.5, XiaomiMiMo | `mtp` | No — CRD default works |
| DeepSeek-V3.2 | `deepseek_mtp` | Yes: `method: "deepseek_mtp"` |
| Qwen3-Next | `qwen3_next_mtp` | Yes: `method: "qwen3_next_mtp"` |

```yaml
# Generic MTP (DeepSeek-V3, Qwen3.5)
speculativeDecoding:
  method: mtp
  numSpeculativeTokens: 1

# Qwen3-Next — requires runtime-specific method
speculativeDecoding:
  method: mtp
  numSpeculativeTokens: 2
  additionalConfig:
    method: "qwen3_next_mtp"

# DeepSeek-V3.2 — requires runtime-specific method
speculativeDecoding:
  method: mtp
  numSpeculativeTokens: 2
  additionalConfig:
    method: "deepseek_mtp"
```

**Example:** [`llm-inference-service-qwen3-mtp.yaml`](llm-inference-service-qwen3-mtp.yaml) — Qwen3-Next on H100

## Examples summary

| Example | Method | Target model | Draft/Speculator | GPU | Replicas |
|---|---|---|---|---|---|
| [Eagle3](llm-inference-service-qwen32b-eagle3.yaml) | eagle3 | Qwen3-32B FP8 | Eagle3 speculator head | H100 | 4 × TP2 |
| [Draft-target (A100)](llm-inference-service-llama8b-draft-target.yaml) | draft_model | Llama-3.1-8B | Llama-3.2-1B | A100 40GB | 4 × TP1 |
| [Draft-target (L40S)](llm-inference-service-llama8b-draft-target-l40s.yaml) | draft_model | Llama-3.1-8B | Llama-3.2-1B | L40S 48GB | 8 × TP1 |
| [N-gram](llm-inference-service-gemma3-ngram.yaml) | ngram | Gemma 3 4B | None | A10G 24GB | 4 × TP1 |
| [MTP](llm-inference-service-qwen3-mtp.yaml) | mtp | Qwen3-Next-80B FP8 | None (built-in heads) | H100 | 2 × TP4 |
| [Medusa](llm-inference-service-vicuna7b-medusa.yaml) | medusa | Vicuna 7B | Medusa heads | H100 | 8 × TP1 |

## Prerequisites

- GPU nodes on the target cluster
- A HuggingFace token secret:
  ```bash
  kubectl create secret generic llm-d-hf-token \
    --from-literal=HF_TOKEN=<your-token> -n <namespace>
  ```
- A Gateway resource (see [`build-and-deploy/`](build-and-deploy/) for examples)
- Access to gated models (Llama requires Meta approval, Qwen3-Next requires Qwen approval)

## Build and deploy

See [`build-and-deploy/BUILD.md`](build-and-deploy/BUILD.md) for instructions on building
a custom llmisvc-controller image and deploying to a cluster.
