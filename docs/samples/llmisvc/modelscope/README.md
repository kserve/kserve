# Download a model from ModelScope

Set the model URI to `modelscope://<owner>/<model>[:revision]`. The storage
initializer downloads the model before the serving container starts. The
following example uses a public model:

```yaml
apiVersion: serving.kserve.io/v1alpha2
kind: LLMInferenceService
metadata:
  name: qwen-modelscope
spec:
  model:
    uri: modelscope://Qwen/Qwen2.5-7B-Instruct
    name: Qwen/Qwen2.5-7B-Instruct
  replicas: 1
  router:
    route: {}
  template:
    containers:
      - name: main
        resources:
          requests:
            cpu: "2"
            memory: 16Gi
            nvidia.com/gpu: "1"
          limits:
            cpu: "4"
            memory: 32Gi
            nvidia.com/gpu: "1"
```

Private models require a Secret containing `MODELSCOPE_API_TOKEN` referenced
by the workload's ServiceAccount. Use a storage-initializer image that includes
`modelscope-hub` and has network access to the model repository.

This supports the base model URI. LoRA adapter URIs and native ModelExpress
sources retain their existing supported schemes.
