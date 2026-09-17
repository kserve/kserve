# Namespace-scoped storage containers

Use a `StorageContainer` to configure the storage initializer for InferenceServices
in one namespace. Unlike a `ClusterStorageContainer`, it can be managed by users
with permission to create StorageContainers in that namespace. A cluster
administrator must install the KServe CRDs first.

The following example selects a custom storage initializer for Hugging Face models.
Create the `team-a` namespace and an `hf-secret` Secret with an `HF_TOKEN` key in
that namespace before applying it. The token is read from the resulting pod's
namespace; the example does not contain credentials.

```yaml
apiVersion: serving.kserve.io/v1alpha1
kind: StorageContainer
metadata:
  name: private-hf-storage
  namespace: team-a
spec:
  supportedUriFormats:
    - prefix: hf://
  workloadType: initContainer
  container:
    name: storage-initializer
    image: kserve/storage-initializer:latest
    env:
      - name: HF_TOKEN
        valueFrom:
          secretKeyRef:
            name: hf-secret
            key: HF_TOKEN
---
apiVersion: serving.kserve.io/v1beta1
kind: InferenceService
metadata:
  name: private-model
  namespace: team-a
spec:
  predictor:
    model:
      modelFormat:
        name: huggingface
      storageUri: hf://your-organization/your-model
```

Replace the model URI and initializer image with values appropriate for your
installation. The model also requires a compatible ServingRuntime and sufficient
resources, just as any other InferenceService does.

## Selection rules

Without `spec.predictor.storageContainerName`, KServe searches eligible
StorageContainers in the InferenceService's namespace, then
ClusterStorageContainers. A matching namespaced container wins even when a cluster
container supports the same URI prefix. Disabled containers and containers whose
`workloadType` is not `initContainer` are skipped. If neither scope matches,
KServe uses its built-in storage handling where the URI is supported.

To select a specific container, set `spec.predictor.storageContainerName` to
`private-hf-storage`. KServe first looks for that name in the InferenceService's
namespace, then in the cluster scope if the namespaced resource is absent.
A same-named namespaced resource shadows the cluster resource. If that namespaced
resource is disabled, has the wrong workload type, or does not support the URI,
selection fails; it does not fall back to the cluster resource. Deleting the
namespaced resource restores the cluster fallback for subsequent lookups.

`disabled: true` is a top-level field alongside `spec`, not a field inside it.
Avoid overlapping matching containers within the same scope: ordering within a
scope is not a selection contract. Use an explicit name when you need a particular
container.

## Scope and updates

This feature applies to InferenceService storage initialization. It does not add
namespaced storage lookup to LLMInferenceService or local model download jobs.

StorageContainer changes do not trigger an InferenceService rollout. For the
single `storageUri` path, selection occurs when a new pod is admitted; existing
pods keep their initializer configuration. For multiple `storageUris`, selection
occurs during InferenceService reconciliation. Do not rely on a StorageContainer
edit alone to update running workloads.
