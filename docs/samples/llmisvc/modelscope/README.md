# Download an LLM from ModelScope

`LLMInferenceService` supports `modelscope://owner/model[:revision]` in
`spec.model.uri`. The default storage initializer downloads the model to
`/mnt/models` before the inference container starts. Omitting `:revision` uses
the repository default revision.

With KServe's LLMInferenceService components, a Gateway, and an NVIDIA GPU
available, deploy the public Qwen model:

```bash
kubectl apply -f llm-inference-service-modelscope.yaml
kubectl logs deployment/qwen3-modelscope-kserve -c storage-initializer
```

The initializer needs network access to ModelScope. The same URI also works for
prefill and multi-node workloads. Tokenizer containers download only tokenizer
and configuration files.

## Private repositories

Create a Secret in the service's namespace with the `MODELSCOPE_API_TOKEN` key
and reference it from the workload's ServiceAccount:

```bash
kubectl create secret generic modelscope-token \
  --from-literal=MODELSCOPE_API_TOKEN="$MODELSCOPE_API_TOKEN"
```

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: modelscope
secrets:
  - name: modelscope-token
```

Set `spec.template.serviceAccountName: modelscope` on the LLMInferenceService.
For a separate prefill workload, set
`spec.prefill.template.serviceAccountName: modelscope` as well. KServe injects
the token into the storage initializer through a Secret reference.

`spec.storageInitializer.enabled: false` disables automatic downloading.
ModelScope support here applies to the base model; LoRA adapters continue to
accept `hf://`, `s3://`, and `pvc://`. ModelExpress native mode accepts only
Hugging Face and S3 sources.
