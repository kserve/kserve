# LocalModelCache Credential Support

## Overview

The `LocalModelCache` CRD now supports specifying credentials for downloading models, providing a consistent experience with `InferenceService`. This eliminates the need to create separate `ClusterStorageContainer` CRDs for model caching.

## Changes

### Before

Previously, caching a model required:
1. Creating a `ClusterStorageContainer` CRD with `workloadType: localModelDownloadJob`
2. Embedding credentials (e.g., HF token) in the container spec via env vars from secrets
3. Creating secrets in the `kserve-localmodel-jobs` namespace

### After

Now you can specify credentials directly in the `LocalModelCache` CRD using the same patterns as `InferenceService`:
- `serviceAccountName`: Reference a service account with attached secrets
- `storage.key`: Reference a specific key in the storage-config secret
- `storage.parameters`: Inline parameters for storage configuration

## Credential Specification Methods

### Method 1: ServiceAccountName

Use a service account that has secrets attached containing storage credentials.

```yaml
apiVersion: serving.kserve.io/v1alpha1
kind: LocalModelCache
metadata:
  name: llama3-8b
spec:
  sourceModelUri: "hf://meta-llama/Meta-Llama-3-8B"
  modelSize: 10Gi
  nodeGroups:
    - workers
  serviceAccountName: hf-downloader  # SA with HF token secret attached
```

Create the service account with HuggingFace token:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: hf-secret
  namespace: kserve-localmodel-jobs
type: Opaque
stringData:
  HF_TOKEN: <your-hf-token>
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: hf-downloader
  namespace: kserve-localmodel-jobs
secrets:
  - name: hf-secret
```

### Method 2: Storage Key Reference

Reference a specific key in the storage-config secret.

```yaml
apiVersion: serving.kserve.io/v1alpha1
kind: LocalModelCache
metadata:
  name: llama3-8b
spec:
  sourceModelUri: "hf://meta-llama/Meta-Llama-3-8B"
  modelSize: 10Gi
  nodeGroups:
    - workers
  storage:
    key: hf-credentials  # Key in storage-config secret
```

The storage-config secret should be in the `kserve-localmodel-jobs` namespace:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: storage-config
  namespace: kserve-localmodel-jobs
type: Opaque
stringData:
  hf-credentials: |
    {
      "type": "hf",
      "token": "<your-hf-token>"
    }
```

### Method 3: Inline Parameters

Provide storage parameters inline for additional configuration.

```yaml
apiVersion: serving.kserve.io/v1alpha1
kind: LocalModelCache
metadata:
  name: s3-model
spec:
  sourceModelUri: "s3://mybucket/mymodel"
  modelSize: 5Gi
  nodeGroups:
    - workers
  storage:
    key: my-s3-key
    parameters:
      type: s3
      region: us-west-2
```

## Container Spec Configuration

The download job container spec is now derived from the `storageInitializer` config in the `inferenceservice-config` ConfigMap:

```yaml
storageInitializer: |-
  {
    "image": "kserve/storage-initializer:latest",
    "cpuRequest": "100m",
    "cpuLimit": "1",
    "memoryRequest": "200Mi",
    "memoryLimit": "1Gi"
  }
```

## Backward Compatibility

- Existing `ClusterStorageContainer` with `workloadType: localModelDownloadJob` continues to work
- If a matching `ClusterStorageContainer` is found, it takes precedence
- Existing `LocalModelCache` resources without credentials continue to work with default storage-initializer

## LocalModelCache CRD Fields

| Field | Type | Description |
|-------|------|-------------|
| `sourceModelUri` | string | Required. URI of the model to cache |
| `modelSize` | resource.Quantity | Required. Size of the model |
| `nodeGroups` | []string | Required. Node groups to cache the model on |
| `serviceAccountName` | string | Optional. Service account for credential lookup |
| `storage` | LocalModelStorageSpec | Optional. Storage configuration for credentials |

### LocalModelStorageSpec

| Field | Type | Description |
|-------|------|-------------|
| `key` | *string | Optional. Storage key in the secret |
| `parameters` | *map[string]string | Optional. Override parameters for storage config |

## Example: Complete HuggingFace Model Caching Setup

1. Create the namespace for download jobs (if not exists):
```bash
kubectl create namespace kserve-localmodel-jobs
```

2. Create a secret with your HuggingFace token:
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: hf-secret
  namespace: kserve-localmodel-jobs
type: Opaque
stringData:
  HF_TOKEN: hf_xxxx
```

3. Create a service account that references the secret:
```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: hf-downloader
  namespace: kserve-localmodel-jobs
secrets:
  - name: hf-secret
```

4. Create the LocalModelNodeGroup:
```yaml
apiVersion: serving.kserve.io/v1alpha1
kind: LocalModelNodeGroup
metadata:
  name: workers
spec:
  storageLimit: 100Gi
  persistentVolumeClaimSpec:
    accessModes:
      - ReadWriteOnce
    resources:
      requests:
        storage: 100Gi
    storageClassName: local-storage
  persistentVolumeSpec:
    accessModes:
      - ReadWriteOnce
    capacity:
      storage: 100Gi
    local:
      path: /models
    storageClassName: local-storage
    nodeAffinity:
      required:
        nodeSelectorTerms:
          - matchExpressions:
              - key: node-type
                operator: In
                values:
                  - gpu
```

5. Create the LocalModelCache with credentials:
```yaml
apiVersion: serving.kserve.io/v1alpha1
kind: LocalModelCache
metadata:
  name: meta-llama3-8b-instruct
spec:
  sourceModelUri: "hf://meta-llama/meta-llama-3-8b-instruct"
  modelSize: 10Gi
  nodeGroups:
    - workers
  serviceAccountName: hf-downloader
```

6. Deploy an InferenceService using the cached model:
```yaml
apiVersion: serving.kserve.io/v1beta1
kind: InferenceService
metadata:
  name: huggingface-llama3
spec:
  predictor:
    model:
      modelFormat:
        name: huggingface
      storageUri: hf://meta-llama/meta-llama-3-8b-instruct
      resources:
        limits:
          nvidia.com/gpu: "1"
```

