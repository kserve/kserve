# CPU inference with llama.cpp

Serve a GGUF model with the `kserve-llamacpp` runtime and the llama.cpp
OpenAI-compatible API. This example uses an ungated 135M-parameter model and
requests two CPUs and 2 GiB of memory. Larger models need more memory.

This runtime uses `InferenceService`. It does not add llama.cpp support to
`LLMInferenceService`, distributed inference, or KServe's v1/v2 prediction APIs.

## Deploy

Use a KServe release with single-file Hugging Face URI support, or build the
storage initializer from this source tree. Older initializers reject file URIs.

Install KServe, then apply the runtime if your installation does not include it:

```sh
kubectl apply -f config/runtimes/kserve-llamacpp.yaml
kubectl apply -f docs/samples/v1beta1/llamacpp/llamacpp.yaml
kubectl wait --for=condition=Ready inferenceservice/llamacpp-smollm2 --timeout=15m
```

For Helm installations, enable serving runtimes in the `kserve-runtime-configs`
chart with `--set kserve.servingruntime.enabled=true`. The
`kserve.servingruntime.llamacpp` values configure its image, pull secrets, pull
policy, security context, and whether the runtime is disabled.

KServe's storage initializer downloads the single file specified by `storageUri`.
The serving container loads it from `/mnt/models`. Set `--model` to the downloaded
filename; for a subdirectory such as `quantized/model.gguf`, use
`--model /mnt/models/quantized/model.gguf` as two separate arguments. A revision goes after the repository
name: `hf://owner/repository:revision/path/model.gguf`.

For models already in object storage or a PVC, use an `s3://` or `pvc://`
`storageUri` and the corresponding path under `/mnt/models` instead.

## Send a request

Forward the predictor Service in a separate terminal:

```sh
kubectl port-forward service/llamacpp-smollm2-predictor 8080:80
```

```sh
curl http://localhost:8080/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{"model":"llamacpp-smollm2","messages":[{"role":"user","content":"Say hello."}],"max_tokens":32}'
```

Add `"stream":true` for server-sent events. `/v1/completions` accepts text prompts,
`/health` reports readiness, and `/metrics` exposes Prometheus metrics. The model
name is the `InferenceService` name. Runtime arguments, including the default
2048-token context size, can be overridden with `spec.predictor.model.args`.
Use separate list items for a flag and its value; llama.cpp does not accept
`--flag=value` syntax.
