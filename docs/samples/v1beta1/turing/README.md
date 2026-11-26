# Turing Engine

[Turing Engine](https://github.com/intutic/turing) serves Hugging Face checkpoints through an OpenAI-compatible
`/v1` API. The `kserve-turingserver` ClusterServingRuntime is opt-in: select it with `runtime: kserve-turingserver`,
and the storage initializer downloads `storageUri` to `/mnt/models`, which the runtime serves under the
InferenceService's name.

It was tested in both deployment modes, with KServe v0.21.0 on GKE and one L4 node: Standard, which the sample sets,
and Knative, the default of KServe's Knative-mode install (remove the `serving.kserve.io/deploymentMode` annotation
to use the cluster's default).

The storage initializer needs more than its default 1Gi memory limit to download this 3 GB checkpoint: at 1Gi it
was killed for running out of memory. 4Gi worked. Set it with `--set kserve.storage.resources.limits.memory=4Gi` on
the `kserve-resources` chart, or as `memoryLimit` under `storageInitializer` in the `inferenceservice-config`
ConfigMap followed by a restart of `kserve-controller-manager`.

In Standard mode:

```bash
kubectl apply -f turing.yaml
kubectl wait --for=condition=Ready inferenceservice/qwen25-1-5b-instruct --timeout=900s
kubectl port-forward svc/qwen25-1-5b-instruct-predictor 8080:80 &
curl -s localhost:8080/v1/chat/completions -H 'Content-Type: application/json' \
  -d '{"model": "qwen25-1-5b-instruct", "messages": [{"role": "user", "content": "Name three primary colours."}], "max_tokens": 48}'
```

In Knative mode, send the request through the ingress gateway with the InferenceService's host
(`kubectl get inferenceservice qwen25-1-5b-instruct -o jsonpath='{.status.url}'`).

`/health` answers once the model is loaded and the server's kernels are warmed up, and `/metrics` is Prometheus
text. The image is built for CUDA GPUs; without one it falls back to the CPU, which is slow. `hf://` storage
URIs need KServe 0.15 or later.
