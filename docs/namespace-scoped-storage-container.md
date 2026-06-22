# Design: Namespace-Scoped StorageContainer

**Issue:** [#5482](https://github.com/kserve/kserve/issues/5482)
**Branch:** `StorageContainer`
**API Group:** `serving.kserve.io/v1alpha1`
**Status:** Implementation complete

---

## 1. Problem

KServe's existing `ClusterStorageContainer` CRD is cluster-scoped only. This creates three problems in multi-tenant environments:

1. **Privilege escalation** — Creating or modifying storage container configuration requires `cluster-admin`. Namespace owners cannot self-serve.
2. **No isolation** — All namespaces share the same cluster-wide storage configurations. Teams with conflicting requirements (different HuggingFace tokens, different init container images) cannot coexist.
3. **Secret cross-namespace reference** — A cluster-scoped resource cannot naturally reference a `Secret` that lives in one team's namespace.

---

## 2. Solution

Add a namespace-scoped `StorageContainer` CRD that mirrors the spec of `ClusterStorageContainer` but is owned by namespace admins. The resolution logic follows the same pattern as Kubernetes `Role` vs `ClusterRole`:

- Namespace-scoped `StorageContainer` takes precedence over cluster-scoped `ClusterStorageContainer`
- Cluster-scoped `ClusterStorageContainer` acts as the fallback / default
- Built-in defaults are the last resort (no custom container found at all)

---

## 3. API

### 3.1 New CRD: `StorageContainer` (Namespaced)

```yaml
apiVersion: serving.kserve.io/v1alpha1
kind: StorageContainer
metadata:
  name: private-hf-storage
  namespace: team-a                 # namespace-scoped
spec:
  supportedUriFormats:
  - prefix: "hf://"
  workloadType: initContainer       # default
  container:
    name: storage-initializer
    image: kserve/storage-initializer:latest
    env:
    - name: HF_TOKEN
      valueFrom:
        secretKeyRef:
          name: hf-secret           # secret in the same namespace — works!
          key: HF_TOKEN
    resources:
      requests:
        cpu: 100m
        memory: 100Mi
      limits:
        cpu: "1"
        memory: 1Gi
```

### 3.2 Existing CRD: `ClusterStorageContainer` (unchanged)

```yaml
apiVersion: serving.kserve.io/v1alpha1
kind: ClusterStorageContainer
metadata:
  name: hf-default                  # no namespace — cluster-scoped
spec:
  supportedUriFormats:
  - prefix: "hf://"
  workloadType: initContainer
  container:
    name: storage-initializer
    image: kserve/storage-initializer:latest
```

### 3.3 Go type definitions

Both types share an identical `StorageContainerSpec`. Only the scope annotation differs:

```go
// Cluster-scoped (existing)
// +kubebuilder:resource:scope="Cluster"
type ClusterStorageContainer struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`
    Spec     StorageContainerSpec `json:"spec,omitempty"`
    Disabled *bool                `json:"disabled,omitempty"`
}

// Namespace-scoped (new)
// +kubebuilder:resource:scope="Namespaced"`
type StorageContainer struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`
    Spec     StorageContainerSpec `json:"spec,omitempty"`
    Disabled *bool                `json:"disabled,omitempty"`
}
```

### 3.4 Shared spec

```go
type StorageContainerSpec struct {
    Container                  corev1.Container     `json:"container"`
    SupportedUriFormats        []SupportedUriFormat `json:"supportedUriFormats"`
    WorkloadType               WorkloadType         `json:"workloadType,omitempty"`
    SupportsMultiModelDownload *bool                `json:"supportsMultiModelDownload,omitempty"`
}
```

---

## 4. InferenceService — No API change

The existing `storageContainerName` field on `PredictorSpec` is reused as-is:

```yaml
apiVersion: serving.kserve.io/v1beta1
kind: InferenceService
metadata:
  name: my-model
  namespace: team-a
spec:
  predictor:
    storageContainerName: "private-hf-storage"   # optional: pin by name
    model:
      modelFormat:
        name: huggingface
      storageUri: hf://meta-llama/Llama-3.2-1B
```

If `storageContainerName` is omitted, the system auto-matches by URI prefix/regex.

---

## 5. Lookup Precedence

### 5.1 Auto-match (no `storageContainerName` set)

```
1. List StorageContainers in InferenceService's namespace
   Filter: not disabled, workloadType == initContainer, URI supported
   → First match returned

2. List ClusterStorageContainers (cluster-wide)
   Filter: same criteria
   → First match returned

3. No match → use built-in default storage initializer
```

### 5.2 Named lookup (`storageContainerName` is set)

```
1. GET StorageContainer "<name>" in InferenceService's namespace
   → Found: run eligibility checks → return spec (or error on failure)
   → 404:   fall through

2. GET ClusterStorageContainer "<name>" (cluster-wide)
   → Found: run eligibility checks → return spec (or error on failure)
   → 404:   return "not found" error
```

Eligibility checks (both paths):
- `disabled != true`
- `workloadType == initContainer`
- URI matches `supportedUriFormats` (prefix or regex)

If a resource is found but fails an eligibility check, an error is returned immediately — no silent fallback.

---

## 6. Code Changes

### 6.1 New type + registration
**File:** `pkg/apis/serving/v1alpha1/storage_container_types.go`

- Added `StorageContainer` struct with `scope="Namespaced"`
- Added `StorageContainerList`
- Added `IsDisabled()` method on `StorageContainer`
- Registered both in `SchemeBuilder`

### 6.2 Webhook lookup logic
**File:** `pkg/webhook/admission/pod/storage_initializer_injector.go`

`GetStorageContainerSpec` — renamed `client` param to `c` (un-shadows the package import), added `namespace string` param, restructured flow:

```
Named? → GetStorageContainerSpecByName(namespace, name)
Auto?  → list namespaced → list cluster → nil
```

`GetStorageContainerSpecByName` — added `namespace string` param, new two-phase lookup:

```
namespace != "" → GET StorageContainer{namespace, name}
                  found → check eligibility → return
                  404   → fall through
               → GET ClusterStorageContainer{name}
                  found → check eligibility → return
                  404   → error
```

### 6.3 Namespace propagation through controllers
**Files:** `components/explainer.go`, `components/predictor.go`, `components/transformer.go`, `utils/utils.go`

`ValidateStorageURI` and `addStorageInitializerAnnotations` received a `namespace string` parameter. All call sites pass `isvc.Namespace`. Pod webhook call site passes `pod.Namespace`.

### 6.4 RBAC
**Files:** `config/rbac/role.yaml`, `charts/kserve-resources/files/kserve/resources.yaml`

Added `storagecontainers` to the `serving.kserve.io` resource list so the controller can `get`, `list`, and `watch` namespace-scoped resources.

### 6.5 Generated artifacts (via `make generate`)
- `pkg/apis/serving/v1alpha1/zz_generated.deepcopy.go`
- `pkg/client/clientset/versioned/typed/serving/v1alpha1/storagecontainer.go`
- `pkg/client/clientset/versioned/typed/serving/v1alpha1/fake/fake_storagecontainer.go`
- `pkg/client/informers/externalversions/serving/v1alpha1/storagecontainer.go`
- `pkg/client/listers/serving/v1alpha1/storagecontainer.go`
- `config/crd/full/serving.kserve.io_storagecontainers.yaml`
- `config/crd/minimal/serving.kserve.io_storagecontainers.yaml`
- `charts/kserve-crd/templates/serving.kserve.io_storagecontainers.yaml`
- `charts/kserve-crd-minimal/templates/serving.kserve.io_storagecontainers.yaml`
- `pkg/openapi/openapi_generated.go`, `pkg/openapi/swagger.json`
- Python SDK: `python/kserve/kserve/models/v1alpha1_storage_container.py`

---

## 7. Why `secretKeyRef` Works

A namespace-scoped `StorageContainer` allows teams to reference secrets in their own namespace:

```yaml
env:
- name: HF_TOKEN
  valueFrom:
    secretKeyRef:
      name: hf-secret      # lives in the same namespace as the StorageContainer
      key: HF_TOKEN
```

KServe does not resolve the secret reference itself — it copies the `secretKeyRef` entry verbatim into the init container spec via `mergeContainerSpecs()`. Kubernetes resolves the reference at pod startup time in the pod's namespace. The secret, the `StorageContainer`, and the `InferenceService` all live in the same namespace.

---

## 8. Backward Compatibility

- All existing `ClusterStorageContainer` resources work without any changes.
- The `ClusterStorageContainer` API is unchanged.
- The `InferenceService` API is unchanged — `storageContainerName` now also resolves namespace-scoped resources transparently.
- If no `StorageContainer` exists in a namespace, the system falls back to `ClusterStorageContainer` exactly as before.

---

## 9. Testing

### 9.1 Unit test cases

| Test | Expected result |
|------|----------------|
| Auto-match: namespaced SC exists and matches URI | Returns namespaced SC spec |
| Auto-match: no namespaced SC, cluster CSC matches | Returns cluster CSC spec |
| Auto-match: namespaced SC exists but disabled | Skips it, falls back to cluster CSC |
| Named lookup: SC exists in namespace | Returns namespaced SC spec |
| Named lookup: SC not in namespace, CSC exists | Returns cluster CSC spec |
| Named lookup: SC found but URI not supported | Returns error (no fallback) |
| Named lookup: neither SC nor CSC found | Returns "not found" error |

### 9.2 Manual cluster test

```bash
# 1. Install new CRD
kubectl apply -f config/crd/full/serving.kserve.io_storagecontainers.yaml

# 2. Create namespace and secret
kubectl create namespace team-a
kubectl create secret generic hf-secret \
  --from-literal=HF_TOKEN=<your_token> \
  -n team-a

# 3. Create namespace-scoped StorageContainer
kubectl apply -f - <<EOF
apiVersion: serving.kserve.io/v1alpha1
kind: StorageContainer
metadata:
  name: private-hf-storage
  namespace: team-a
spec:
  supportedUriFormats:
  - prefix: "hf://"
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
EOF

# 4. Create InferenceService
kubectl apply -f - <<EOF
apiVersion: serving.kserve.io/v1beta1
kind: InferenceService
metadata:
  name: test-hf-model
  namespace: team-a
spec:
  predictor:
    model:
      modelFormat:
        name: huggingface
      storageUri: hf://meta-llama/Llama-3.2-1B
EOF

# 5. Verify HF_TOKEN was injected into the init container
kubectl get pod -n team-a -l serving.kserve.io/inferenceservice=test-hf-model -o name | \
  xargs -I{} kubectl get {} -n team-a \
  -o jsonpath='{.spec.initContainers[0].env}' | jq .
```

### 9.3 Verify precedence (namespace overrides cluster)

```bash
# Create a ClusterStorageContainer that also matches hf://
kubectl apply -f - <<EOF
apiVersion: serving.kserve.io/v1alpha1
kind: ClusterStorageContainer
metadata:
  name: hf-cluster-default
spec:
  supportedUriFormats:
  - prefix: "hf://"
  workloadType: initContainer
  container:
    name: storage-initializer
    image: kserve/storage-initializer:cluster-version
EOF

# The pod's init container image should be :latest (from the namespaced SC)
# NOT :cluster-version (from the CSC)
```

---

## 10. Files Changed Summary

| File | Type of change |
|------|---------------|
| `pkg/apis/serving/v1alpha1/storage_container_types.go` | New `StorageContainer` type, `IsDisabled()`, scheme registration |
| `pkg/webhook/admission/pod/storage_initializer_injector.go` | Namespace-first lookup in `GetStorageContainerSpec` and `GetStorageContainerSpecByName` |
| `pkg/controller/v1beta1/inferenceservice/components/explainer.go` | Pass `isvc.Namespace` to `ValidateStorageURI` |
| `pkg/controller/v1beta1/inferenceservice/components/predictor.go` | Pass `isvc.Namespace` through `addStorageInitializerAnnotations` |
| `pkg/controller/v1beta1/inferenceservice/components/transformer.go` | Pass `isvc.Namespace` to `ValidateStorageURI` |
| `pkg/controller/v1beta1/inferenceservice/utils/utils.go` | Add `namespace` param to `ValidateStorageURI` |
| `pkg/controller/v1beta1/inferenceservice/utils/utils_test.go` | Update test call sites |
| `pkg/webhook/admission/pod/storage_initializer_injector_test.go` | Update test call sites |
| `config/rbac/role.yaml` | Add `storagecontainers` resource |
| `charts/kserve-resources/files/kserve/resources.yaml` | Add `storagecontainers` resource |
| Generated files (deepcopy, client, listers, informers, CRDs, OpenAPI, Python SDK) | Auto-generated via `make generate` |
