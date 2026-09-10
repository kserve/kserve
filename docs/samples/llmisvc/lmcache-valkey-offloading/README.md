# LMCache KV Cache Offloading with Valkey

This sample deploys an `LLMInferenceService` using [LMCache](https://docs.lmcache.ai/index.html)
for KV cache offloading, with [Valkey](https://valkey.io/) as the remote, distributed backend.
Valkey is a drop-in, BSD-licensed alternative to Redis: LMCache's `ValkeyConnector` speaks the
same wire protocol, so only the `remote_url` scheme changes from `redis://` to `valkey://`.

See the [KV Cache Offloading with Huggingface vLLM Backend](https://kserve.github.io/website/docs/model-serving/generative-inference/kvcache-offloading)
guide for the full step-by-step walkthrough this sample accompanies.

> This is a different feature from the [`kv-cache-offloading`](../kv-cache-offloading/) sample
> in this directory, which configures KServe's native `workload.kvCacheOffloading` GPU → CPU →
> disk tiering. This sample instead offloads to a remote key-value store via LMCache, which is
> useful for sharing KV cache across multiple inference replicas.

## What this deploys

[`llmisvc-lmcache-valkey.yaml`](llmisvc-lmcache-valkey.yaml) contains:

- A `ConfigMap` with the LMCache configuration, pointing `remote_url` at the Valkey service
- A single-replica `valkey/valkey:9.1.0` `Deployment` and `Service`
- An `LLMInferenceService` running `meta-llama/Llama-3.2-1B-Instruct` with LMCache enabled via
  `--kv-transfer-config`, and the LMCache config mounted from the `ConfigMap`

## Prerequisites

- A Kubernetes cluster with [KServe v0.18.0](https://kserve.github.io/website/docs/getting-started/quickstart-guide) or later installed
- GPU nodes (the sample requests `nvidia.com/gpu: "1"`)
- A Huggingface token secret named `hf-secret` with key `HF_TOKEN` (see the guide's
  [Create Huggingface Secret](https://kserve.github.io/website/docs/model-serving/generative-inference/kvcache-offloading#create-huggingface-secret-hf_token) step)

## Deploy

```bash
kubectl apply -f llmisvc-lmcache-valkey.yaml
```

Wait for the Valkey pod and the `LLMInferenceService` to be ready:

```bash
kubectl get pods -l app=valkey
kubectl get llminferenceservice llama3-lmcache-valkey
```

## Verify

Send an inference request through the OpenAI-compatible route (see the guide's
[Verify the Setup with an Inference Request](https://kserve.github.io/website/docs/model-serving/generative-inference/kvcache-offloading#verify-the-setup-with-an-inference-request)
section), then confirm KV cache entries landed in Valkey:

```bash
kubectl exec deploy/valkey -it -- valkey-cli KEYS '*'
```

You should see keys of the form `meta-llama/Llama-3.2-1B-Instruct@<worker>@<layer>@<hash>@half`,
one per 256-token chunk that LMCache offloaded. Sending the same prompt again should produce a
cache hit in the vLLM logs (`LMCache hit tokens: 256`).
