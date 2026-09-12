# LocalModelNamespaceCache Shared-PVC Import

## Overview

A namespace-scoped `LocalModelNamespaceCache` can import a model **once** onto a
pre-created, shared, read-write-many (RWX) PersistentVolumeClaim instead of fanning
the model out to a per-node hostPath copy on every node in a node group.

When `spec.pvcRef` is set, KServe:

- Runs a single import Job that downloads the model onto the referenced PVC.
- Skips node fan-out entirely: no `LocalModelNode`, per-node PV, or per-node PVC is created.
- Lets every serving replica mount that one imported copy read-only.

This is well suited to environments that already have RWX storage (NFS, CephFS,
Filestore, Azure Files, etc.) and want a single model copy shared across replicas
rather than one copy per node.

The cluster-scoped `LocalModelCache` and the existing `nodeGroups` fan-out path are
unchanged.

## Spec: `nodeGroups` XOR `pvcRef`

`spec.nodeGroups` and `spec.pvcRef` are mutually exclusive, and exactly one must be set:

- Set `nodeGroups` for the per-node hostPath fan-out (existing behavior).
- Set `pvcRef` for the shared-PVC import (this document).

Setting neither or both is rejected by the validating webhook. `pvcRef` is
**immutable** once set; to point at a different claim, create a new cache.

## Requirements for the referenced PVC

The PVC named by `pvcRef` must already exist **in the same namespace as the cache**
and must:

- Use `Filesystem` volume mode (the default).
- Include the `ReadWriteMany` access mode so the import Job can write while serving
  replicas read the same data.
- Request (and, once bound, provide) at least `spec.modelSize` of storage.

The referenced PVC and the imported model data are **user-owned**. KServe never
mutates or deletes the PVC or its contents. Deleting the cache removes only the owned
import Job (via garbage collection); the PVC and the model data are retained.

## Example

`shared-pvc.yaml` in this directory contains a complete example:

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: shared-models
  namespace: my-models
spec:
  accessModes:
    - ReadWriteMany
  volumeMode: Filesystem
  resources:
    requests:
      storage: 20Gi
  storageClassName: nfs-csi
---
apiVersion: serving.kserve.io/v1alpha1
kind: LocalModelNamespaceCache
metadata:
  name: llama3-8b
  namespace: my-models
spec:
  sourceModelUri: "hf://meta-llama/Meta-Llama-3-8B"
  modelSize: 16Gi
  pvcRef: shared-models
```

Apply it:

```bash
kubectl apply -f shared-pvc.yaml
```

## Import and retry lifecycle

- The import Job is deterministic: `<cacheName>-import` (hashed when the name would
  exceed 63 characters). Exactly one Job exists at a time; reconciliation is a no-op
  once it is present.
- The Job runs one completion with one parallel worker and a bounded backoff. It has
  no `ttlSecondsAfterFinished`: succeeded and failed Jobs are **retained** as durable
  evidence and to gate re-import.
- A **failed** Job is not recreated automatically. To retry, delete the failed Job;
  the controller then creates exactly one replacement. Concurrent import Jobs never
  occur.
- The Job records the referenced PVC UID and storage key. Recreating the PVC under
  the same name invalidates the old import and creates a replacement Job.
- A Job with the deterministic name that is not owned by the cache is never adopted
  or deleted; the cache reports `ImportJobConflict` until the collision is removed.

## Condition contract

The cache reports a single positive-polarity `Ready` condition (with
`observedGeneration`). `status.modelCopies` reports `total: 1` with `available` or
`failed` set accordingly; `status.nodeStatus` is empty in shared-PVC mode.

| `Ready` status | Reason | Meaning |
|----------------|--------|---------|
| `False` | `PVCNotFound` | The referenced PVC does not exist in the namespace yet. |
| `False` | `UnsupportedVolumeMode` | The PVC uses `Block` volume mode. |
| `False` | `UnsupportedAccessMode` | The PVC does not include `ReadWriteMany`. |
| `False` | `InsufficientCapacity` | The PVC requested/bound storage is smaller than `modelSize`. |
| `False` | `DestinationConflict` | Another cache already imports this model onto the same PVC. |
| `False` | `ImportPending` | The import Job exists but has not started. |
| `False` | `ImportRunning` | The import Job is running. |
| `False` | `ImportFailed` | The import Job failed; delete the Job to retry. |
| `False` | `ImportJobConflict` | The deterministic import Job name is occupied by a Job the cache does not own. |
| `True`  | `ImportSucceeded` | The model is imported and available for serving. |

`lastTransitionTime` changes only when the `Ready` status (True/False/Unknown)
changes, not when only the reason or message changes.

## Destination ownership

Within a namespace, the tuple `(pvcRef, storageKey)` may be owned by only one cache,
where `storageKey` is derived from `sourceModelUri`. Two caches importing the **same**
model onto the **same** PVC conflict (`DestinationConflict`); two caches importing
**different** models onto the same PVC are allowed (they use different subpaths).

## Readiness ordering and resulting URI

An InferenceService, LLMInferenceService base model, or LoRA adapter is routed to the
shared copy **only after** the cache reaches `Ready: True`. Until then the workload
falls back to its original `storageUri` as if the cache were absent.

Once ready, the served model resolves to:

```
pvc://<pvcRef>/models/<storageKey><subPath>
```

Serving pods mount this `pvc://` URI read-only, so they carry no storage-initializer
transfer container: the model is already present on the shared volume.

## Deferred: authenticated private OCI

Authenticated private OCI import for shared-PVC caches is **not yet supported** and is
deferred to a follow-up. Use `hf://`, `s3://`, `gs://`, or other supported sources
in the meantime.
