# Speculative Decoding with LLMInferenceService

Speculative decoding accelerates LLM inference by proposing multiple candidate tokens
cheaply and verifying them in a single target model forward pass. The output is
mathematically identical to standard decoding — it is a pure latency optimization.

LLMInferenceService provides first-class support via `spec.speculator`:

```yaml
spec:
  speculator:
    model:
      uri: hf://RedHatAI/Qwen3-32B-speculator.eagle3
    config:
      method: eagle3
      num_speculative_tokens: "3"
      draft_tensor_parallel_size: "1"
      max_model_len: "4096"
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
- `model` field required (model download needed)

```yaml
speculator:
  model:
    uri: hf://RedHatAI/Qwen3-32B-speculator.eagle3
  config:
    method: eagle3
    num_speculative_tokens: "3"
    draft_tensor_parallel_size: "1"
```

**Example:** [`llm-inference-service-H100-eagle3.yaml`](llm-inference-service-H100-eagle3.yaml)

### Draft-target

A smaller model from the same family generates draft tokens independently. The classic
approach — no special training needed, works with any compatible model pair that shares
the same vocabulary and tokenizer.

- Draft model is downloaded and co-located alongside the target
- `model` field required (model download needed)
- Lower acceptance rates than Eagle3 since the draft model doesn't see the target's
  internal state

```yaml
speculator:
  model:
    uri: hf://meta-llama/Llama-3.2-1B-Instruct
  config:
    method: draft_model
    num_speculative_tokens: "5"
    draft_tensor_parallel_size: "1"
    max_model_len: "8192"
```

**Examples:**
- [`llm-inference-service-A100-draft-target.yaml`](llm-inference-service-A100-draft-target.yaml) — A100 40GB
- [`llm-inference-service-L40S-draft-target.yaml`](llm-inference-service-L40S-draft-target.yaml) — L40S 48GB

### Medusa

Multi-head decoder trained on top of the target model. Similar to Eagle3 but uses
tree-based attention for candidate verification.

> **Note:** Medusa is supported at the CRD level but practical adoption is limited.
> Pre-trained Medusa heads compatible with vLLM are scarce, and the technique has been
> largely superseded by Eagle3 (better acceptance rates, wider model coverage). We include
> it for completeness but recommend Eagle3 or draft-target for new deployments.

- Requires trained Medusa heads per target model
- `model` field required (model download needed)

```yaml
speculator:
  model:
    uri: hf://FasterDecoding/vllm-medusa-vicuna-7b-v1.3
  config:
    method: medusa
    num_speculative_tokens: "5"
    draft_tensor_parallel_size: "1"
```

### N-gram

Proposes draft tokens by matching n-gram patterns from the input prompt itself. Zero
overhead — no separate model, no extra memory. Effective for input-grounded tasks
like summarization, question-answering, and code completion where the output echoes
the input.

- No `model` field (no model download needed)
- `prompt_lookup_max` and `prompt_lookup_min` control n-gram matching window

```yaml
speculator:
  config:
    method: ngram
    num_speculative_tokens: "4"
    prompt_lookup_max: "5"
    prompt_lookup_min: "2"
```

**Example:** [`llm-inference-service-A10-ngram.yaml`](llm-inference-service-A10-ngram.yaml)

### MTP (Multi-Token Prediction)

Uses the target model's own built-in MTP heads to speculate. No separate model needed,
but the model must have been trained with MTP support.

- No `model` field (no model download needed)
- Only works with models that have native MTP heads

**Runtime-specific method variants:** Some model families require a specific MTP method
name to be passed to vLLM. Set the appropriate `method` value directly in `config`:

| Model family | vLLM method | config value |
|---|---|---|
| DeepSeek-V3, Qwen3.5, XiaomiMiMo | `mtp` | `method: mtp` |
| DeepSeek-V3.2 | `deepseek_mtp` | `method: deepseek_mtp` |
| Qwen3-Next | `qwen3_next_mtp` | `method: qwen3_next_mtp` |

```yaml
# Generic MTP (DeepSeek-V3, Qwen3.5)
speculator:
  config:
    method: mtp
    num_speculative_tokens: "1"

# Qwen3-Next — requires runtime-specific method
speculator:
  config:
    method: qwen3_next_mtp
    num_speculative_tokens: "2"

# DeepSeek-V3.2 — requires runtime-specific method
speculator:
  config:
    method: deepseek_mtp
    num_speculative_tokens: "2"
```

**Example:** [`llm-inference-service-H100-mtp.yaml`](llm-inference-service-H100-mtp.yaml) — Qwen3-Next on H100

This design was chosen to maximize support for MTP but not bind the Kserve CRDs to vLLM / llm-d as the server provider.

## Examples summary

| Example | Method | Target model | Draft/Speculator | GPU | Replicas |
|---|---|---|---|---|---|
| [Eagle3](llm-inference-service-H100-eagle3.yaml) | eagle3 | Qwen3-32B FP8 | Eagle3 speculator head | H100 | 2 × TP2 |
| [Draft-target (A100)](llm-inference-service-A100-draft-target.yaml) | draft_model | Llama-3.1-8B | Llama-3.2-1B | A100 40GB | 4 × TP1 |
| [Draft-target (L40S)](llm-inference-service-L40S-draft-target.yaml) | draft_model | Llama-3.1-8B | Llama-3.2-1B | L40S 48GB | 8 × TP1 |
| [N-gram](llm-inference-service-A10-ngram.yaml) | ngram | Gemma 3 4B | None | A10G 24GB | 4 × TP1 |
| [MTP](llm-inference-service-H100-mtp.yaml) | qwen3_next_mtp | Qwen3-Next-80B FP8 | None (built-in heads) | H100 | 2 × TP4 |
| Medusa | medusa | — | Medusa heads | — | See note above |

## Prerequisites

- GPU nodes on the target cluster
- A HuggingFace token secret (for some examples):
  ```bash
  kubectl create secret generic llm-d-hf-token \
    --from-literal=HF_TOKEN=<your-token> -n <namespace>
  ```
  - Access to gated models (Llama requires Meta approval, Gemma3 requires Google approval)
