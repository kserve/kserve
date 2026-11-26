# Turing Engine

[Turing Engine](https://github.com/intutic/turing) serves Hugging Face checkpoints through an OpenAI-compatible
`/v1` API. The `kserve-turingserver` ClusterServingRuntime is opt-in: select it with `runtime: kserve-turingserver`,
and the storage initializer downloads `storageUri` to `/mnt/models`, which the runtime serves under the
InferenceService's name. The sample sets Standard deployment mode, the mode it was tested in (KServe v0.21.0 on GKE
with one L4 node); Knative mode has not been tried.

```bash
kubectl apply -f turing.yaml
kubectl wait --for=condition=Ready inferenceservice/qwen25-1-5b-instruct --timeout=900s
kubectl port-forward svc/qwen25-1-5b-instruct-predictor 8080:80 &
curl -s localhost:8080/v1/chat/completions -H 'Content-Type: application/json' \
  -d '{"model": "qwen25-1-5b-instruct", "messages": [{"role": "user", "content": "Name three primary colours."}], "max_tokens": 48}'
```

`/health` answers once the model is loaded and the server's kernels are warmed up, and `/metrics` is Prometheus
text. The image is built for CUDA GPUs; without one it falls back to the CPU, which is slow. `hf://` storage
URIs need KServe 0.15 or later.
