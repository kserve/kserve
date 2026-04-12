# Speculative Decoding with LLMInferenceService

## How speculative decoding works

Standard autoregressive LLM inference generates one token at a time. Each token requires a
full forward pass through the model, and the next token can't start until the previous one
finishes. For large models, this sequential process is slow — most of the time is spent
waiting on memory bandwidth to load model weights, not on actual computation.

Speculative decoding breaks this bottleneck by introducing a **draft-then-verify** pattern:

```
Standard autoregressive decoding (1 token per forward pass):

  Target model    ┌──────┐   ┌──────┐   ┌──────┐   ┌──────┐   ┌──────┐
  (70B, slow)     │ "The"│──▶│" cat"│──▶│" sat"│──▶│" on" │──▶│" the"│──▶ ...
                  └──────┘   └──────┘   └──────┘   └──────┘   └──────┘
                  1 pass     1 pass     1 pass     1 pass     1 pass
                  = 5 sequential forward passes for 5 tokens


Speculative decoding (multiple tokens verified per forward pass):

  Draft mechanism ┌──────┐   ┌──────┐   ┌──────┐
  (fast)          │" cat"│──▶│" sat"│──▶│" on" │  proposes 3 candidate tokens
                  └──────┘   └──────┘   └──────┘
                       │          │          │
                       ▼          ▼          ▼
  Target model    ┌─────────────────────────────┐
  (70B, slow)     │  verify all 3 in parallel   │  1 forward pass
                  └─────────────────────────────┘
                       │          │          │
                     ✅ " cat"  ✅ " sat"  ✅ " on"   all accepted → 3 tokens in 1 pass
```

The key insight is that **verifying multiple tokens is nearly as fast as generating one**.
The target model's forward pass is dominated by loading weights from memory (memory-bound),
and checking 3-5 candidate tokens adds minimal compute overhead. If the draft mechanism
predicts well, you get multiple tokens for the cost of one forward pass.

When a draft token is **rejected**, the target model's own prediction is used for that
position, and drafting restarts from there. The output is mathematically identical to
standard decoding — speculative decoding is a pure latency optimization with no quality loss.

### Acceptance rate and speedup

The **acceptance rate** (or acceptance length) is the average number of draft tokens accepted
per verification step. It directly determines the speedup:

```
Effective speedup ≈ acceptance_length / (1 + draft_cost / verify_cost)
```

- Higher acceptance rates → more tokens per forward pass → better speedup
- Faster draft mechanisms → lower draft_cost → better speedup
- Acceptance rates are workload-dependent: code and math (predictable patterns) see higher
  acceptance than creative writing or translation

## Two approaches to speculative decoding

### Approach 1: Eagle3 speculator heads

[Eagle3](https://github.com/SafeAILab/EAGLE) (Extrapolation Algorithm for Greater Language-model
Efficiency) trains a lightweight head (~2B params) that sits on top of the target model and
predicts future tokens using the target's own hidden states.

```
                    Target model (frozen, serves requests normally)
                    ┌─────────────────────────────────────────────┐
  Input ──────────▶ │  Layer 1 ──▶ Layer 2 ──▶ ... ──▶ Layer N   │──▶ Output
                    └─────────────┬───────────┬───────────┬───────┘
                                  │           │           │
                          low-level      mid-level     high-level
                          features       features      features
                                  │           │           │
                                  ▼           ▼           ▼
                              ┌───────────────────────────────┐
                              │     Eagle3 speculator head    │
                              │   (~2B params, fuses features │
                              │    from multiple layers)      │
                              └───────────┬───────────────────┘
                                          │
                                          ▼
                                  draft token proposals
                                  (k=3: "cat", "sat", "on")
```

**How it works:**
- Eagle3 fuses hidden states from low, mid, and high layers of the target model
- These fused features, combined with token IDs, are fed into the small draft head
- The draft head autoregressively generates k candidate tokens
- The target model verifies all k candidates in a single forward pass

**Characteristics:**
- Highest acceptance rates (~2.1-2.5 tokens/step at k=3) because the speculator
  directly sees the target model's internal representations
- Requires a purpose-trained speculator head per target model
- The speculator head is co-located on the same GPU(s) as the target model
- Adds ~4 GB of memory overhead (negligible for large models)

### Approach 2: Draft-target model pairs

The classic approach uses a smaller model from the same family as an independent draft model.
Both models share the same vocabulary and tokenizer, but the draft model runs independently.

```
  Input ──────────────────────────────┐
                                      │
                                      ▼
                              ┌───────────────┐
                              │  Draft model  │
                              │  (1B, fast)   │  generates k tokens autoregressively
                              └───────┬───────┘
                                      │
                          draft tokens: "cat", "sat", "on"
                                      │
                                      ▼
                              ┌───────────────┐
                              │ Target model  │
                              │  (8B, slow)   │  verifies all k tokens in 1 forward pass
                              └───────┬───────┘
                                      │
                              accepted: "cat" ✅, "sat" ✅, "on" ✅
```

**How it works:**
- The draft model generates k candidate tokens autoregressively (k separate forward passes,
  but each is very fast because the model is small)
- The target model verifies all k candidates in a single forward pass
- Accepted tokens are emitted; on rejection, the target's prediction is used and drafting
  restarts

**Characteristics:**
- Works with any compatible model pair — no special training needed
- The draft model maintains its own separate KV cache (additional memory)
- Lower acceptance rates than Eagle3 because the draft model doesn't see the target's
  internal state — it's an independent model making independent predictions
- Smaller draft models (1B) often outperform larger ones (3B, 8B) because the drafting
  speed advantage outweighs the marginal acceptance rate improvement

### Comparison summary

```
                    Eagle3 Speculator              Draft-Target Model
                    ──────────────────             ──────────────────
  Training needed?  Yes (per target model)         No (use existing models)
  Acceptance rate   Higher (~2.1-2.5 tokens/step)  Lower (~1.5-2.0 tokens/step)
  Memory overhead   ~4 GB (shared KV cache)        Model weights + separate KV cache
  Compatibility     Needs trained head per model   Any same-family model pair
  Best for          Production (max throughput)     Experimentation / flexibility
```

---

This directory contains two speculative decoding examples using the LLMInferenceService CRD.
Both target H100 80GB GPUs with the same total GPU budget (8 GPUs), but demonstrate different
speculative decoding strategies and scaling patterns.

## Why these examples

These examples are designed to showcase both approaches, and to contrast two scaling strategies
on the same 8-GPU budget:

| | Eagle3 | Draft-Target |
|---|---|---|
| **Target model** | Qwen3-32B (FP8) | Llama-3.1-8B (BF16) |
| **Draft mechanism** | Eagle3 speculator head (2B, BF16) | Llama-3.2-1B (BF16) |
| **GPUs per replica** | 2 (TP=2) | 1 |
| **Replicas** | 4 | 8 |
| **Total GPUs** | 8 | 8 |
| **Scaling pattern** | Larger model, moderate scale-out | Smaller model, max scale-out |

## Example 1: Eagle3 — Qwen3-32B FP8 (4 replicas × TP=2)

**File:** `llm-inference-service-qwen32b-eagle3.yaml`

| Component | Model | Precision | Size |
|-----------|-------|-----------|------|
| Target | [RedHatAI/Qwen3-32B-FP8-dynamic](https://huggingface.co/RedHatAI/Qwen3-32B-FP8-dynamic) | FP8 | ~33 GB |
| Speculator | [RedHatAI/Qwen3-32B-speculator.eagle3](https://huggingface.co/RedHatAI/Qwen3-32B-speculator.eagle3) | BF16 | ~4 GB |

### Why FP8 for the target

H100s have native FP8 tensor cores. At 32B+ parameters, memory bandwidth is the bottleneck —
FP8 halves memory and doubles matrix-multiply throughput with lossless accuracy (100.1% recovery
on OpenLLM v1). This is where FP8 provides meaningful gains.

### Why BF16 for the speculator

The Eagle3 head is only ~2B params (~4 GB). Quantizing it would save ~2 GB but risks degrading
acceptance rates, which directly reduces the speculative decoding speedup. Not worth the tradeoff.

### Why TP=2

Qwen3-32B has 64 attention heads and 8 KV heads — both divisible by 2. With FP8, total model
weight is ~37 GB (target + speculator). At TP=2, each GPU holds ~18.5 GB of weights, leaving
~59 GB for KV cache — plenty of headroom. TP=4 would work but wastes GPUs; TP=1 is tight.

### Why 4 replicas

Speculative decoding provides the highest speedup at low per-replica request rates (where
acceptance rates are best). Spreading load across 4 replicas behind KServe's scheduler keeps
each replica in its optimal operating region.

### Deploy and test

```bash
kubectl apply -f llm-inference-service-qwen32b-eagle3.yaml

kubectl port-forward -n greg svc/qwen32b-eagle3-speculative 8000:8000

curl http://localhost:8000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "RedHatAI/Qwen3-32B-FP8-dynamic",
    "messages": [{"role": "user", "content": "Explain speculative decoding in 3 sentences."}],
    "temperature": 0.6,
    "top_p": 0.95,
    "max_tokens": 256
  }'
```

## Example 2: Draft-Target — Llama 3.1 8B + Llama 3.2 1B (8 replicas × 1 GPU)

**File:** `llm-inference-service-llama8b-draft-target.yaml`

| Component | Model | Precision | Size |
|-----------|-------|-----------|------|
| Target | [meta-llama/Llama-3.1-8B-Instruct](https://huggingface.co/meta-llama/Llama-3.1-8B-Instruct) | BF16 | ~16 GB |
| Draft | [meta-llama/Llama-3.2-1B-Instruct](https://huggingface.co/meta-llama/Llama-3.2-1B-Instruct) | BF16 | ~2.4 GB |

### Why BF16 (no quantization)

At 8B parameters, the model is compute-bound, not memory-bound on H100. FP8 only gives ~11%
throughput improvement at this scale — not worth the added complexity. The 1B draft model is
too small for quantization to matter at all.

### Why 1 GPU per replica (no TP)

Both models fit on a single H100 with ~59 GB free for KV cache. No inter-GPU communication
means lowest possible latency. Simpler scheduling — each replica is a single pod on a single GPU.

### Why 8 replicas

Maximum horizontal scale-out on the same 8-GPU budget. KServe's scheduler distributes requests
across 8 independent replicas, keeping each at low utilization where draft-target acceptance
rates are highest.

### Why Llama 3.2 1B as draft (not 3B or 8B)

Smaller draft models outperform larger ones for speculative decoding. The 1B model proposes
tokens faster (lower drafting latency), and the acceptance rate difference vs 3B is small
enough that the speed advantage wins. Benchmarks show Llama 70B + 1B draft achieves ~2.3x
speedup vs ~1.6x with an 8B draft.

### Deploy and test

```bash
kubectl apply -f llm-inference-service-llama8b-draft-target.yaml

kubectl port-forward -n greg svc/llama8b-draft-target-speculative 8000:8000

curl http://localhost:8000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "meta-llama/Llama-3.1-8B-Instruct",
    "messages": [{"role": "user", "content": "Write a Python function to check if a number is prime."}],
    "temperature": 0,
    "max_tokens": 256
  }'
```

## Prerequisites

- 8x H100 80GB GPUs
- A HuggingFace token secret pre-created in the namespace:
  ```bash
  kubectl create secret generic llm-d-hf-token \
    --from-literal=HF_TOKEN=<your-token> \
    -n greg
  ```
- Access to gated models (Llama requires Meta approval)

## Hardware sizing reference

### Memory budget per GPU

```
GPU total memory:                80 GB  (H100 SXM)
- Model weights (per GPU):      W / TP  (W = total weight size, TP = tensor parallel size)
- Draft/speculator weights:     D / TP  (co-located, split across TP ranks)
- CUDA kernels + overhead:      ~2 GB
= Remaining for KV cache
```

### KV cache per token

```
KV per token = 2 (K+V) × num_kv_heads × head_dim × num_layers × dtype_bytes
```

| Model | KV heads | Layers | Head dim | Dtype | KV/token |
|-------|----------|--------|----------|-------|----------|
| Qwen3-32B | 8 | 64 | 128 | FP8/BF16 | 2.0 MB |
| Llama-3.1-8B | 8 | 32 | 128 | BF16 | 1.0 MB |
| Llama-3.2-1B | 8 | 16 | 64 | BF16 | 0.25 MB |

### Tensor parallelism selection

TP must evenly divide both the number of attention heads and KV heads.

| Model | Attn heads | KV heads | Valid TP | Chosen | Rationale |
|-------|-----------|----------|---------|--------|-----------|
| Qwen3-32B FP8 | 64 | 8 | 1,2,4,8 | 2 | 33 GB FP8 fits on 2 GPUs with ~59 GB/GPU for KV cache |
| Llama-3.1-8B BF16 | 32 | 8 | 1,2,4,8 | 1 | 18 GB total fits on 1 GPU with ~59 GB for KV cache |

### Quantization decision guide (H100)

| Model size | Memory-bound? | FP8 benefit | Recommendation |
|-----------|---------------|-------------|----------------|
| ≤8B | No | ~11% throughput | BF16 — simplicity wins |
| 14-32B | Yes | ~2x throughput, ~50% memory | FP8 — significant gains |
| 70B+ | Very | ~2x throughput, enables fewer GPUs | FP8 — near-mandatory |

## Extension points / first-class support

The current approach works but relies on pass-through env vars. These are areas where KServe
could provide first-class support for speculative decoding:

1. **`parallelism.tensor` on single-node deployments** — The `spec.parallelism.tensor` field
   exists in the CRD but is only templated into multi-node configs. For single-node multi-GPU,
   `--tensor-parallel-size` must be passed via `VLLM_ADDITIONAL_ARGS`. The single-node template
   (`kserve-config-llm-template`) could template this field the same way the multi-node
   templates do.

2. **First-class speculative decoding spec** — A dedicated `spec.speculative` field (with
   `model`, `method`, `numTokens`, `draftTensorParallelSize`, etc.) would provide CRD-level
   validation, better discoverability, and consistent configuration across runtimes. Currently
   tracked as [#3800](https://github.com/kserve/kserve/issues/3800).

3. **Draft model storage initialization** — The storage initializer only downloads the primary
   model. The draft/speculator model is fetched by vLLM at runtime via `HF_TOKEN`. A dedicated
   init for the draft model would improve cold-start times and allow pre-caching on PVCs.
