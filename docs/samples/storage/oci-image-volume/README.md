# OCI Image Volume storage for InferenceService

This sample shows how to mount a model container image directly as a Kubernetes
[ImageVolume](https://kubernetes.io/docs/concepts/storage/volumes/#image) using
the `oci+native://` storageUri scheme (KServe issue [#4083](https://github.com/kserve/kserve/issues/4083)).

## Prerequisites

| Requirement | Minimum version |
|---|---|
| Kubernetes | 1.33 (ImageVolume beta, gate on by default) |
| Kubernetes (with manual gate) | 1.31–1.32 + `--feature-gates=ImageVolume=true` on apiserver and kubelet |
| Container runtime | containerd ≥ 2.0 or CRI-O ≥ 1.31 |
| KServe | this branch or later |

## How to apply

1. Replace the `storageUri` in `inference_service.yaml` with your OCI model image reference.
   The image must contain model files at `/mnt/models`.

2. Apply the manifest:
   ```bash
   kubectl apply -f inference_service.yaml
   ```

## How to verify

Check that the InferenceService is created:
```bash
kubectl get inferenceservice sklearn-oci-native -n kserve-test
```

Inspect the materialized pod spec. The admission webhook should have added an
`image` volume and a matching read-only mount on `kserve-container`:
```bash
kubectl describe pod -n kserve-test -l serving.kserve.io/inferenceservice=sklearn-oci-native
```

Look for a volume stanza like:
```
Volumes:
  mnt-models:
    Type:       Image (an OCI container image)
    Reference:  ghcr.io/my-org/my-sklearn-model:v1
    ...
```

And a container mount:
```
    Mounts:
      /mnt/models from mnt-models (ro)
```

## Global default vs explicit scheme

You can configure `ociModelMode: "native"` in the `inferenceservice-config` ConfigMap
to make all `oci://` URIs use native mounting by default.  The `oci+native://` scheme
is an explicit override that bypasses the global setting — useful when you want a
single service to use native mode while the cluster default remains `modelcar`.

See the commented-out ConfigMap snippet in `inference_service.yaml` for the operator
configuration change.

## When to use this vs alternatives

| Mode | URI scheme | How it works | When to use |
|---|---|---|---|
| `native` | `oci+native://` or `oci://` (if default) | Kubernetes ImageVolume — no sidecar | K8s ≥ 1.33, model image already in OCI format |
| `modelcar` | `oci://` (default) | Sidecar container sharing `/mnt/models` | Any K8s version, existing modelcar images |
| *(planned)* `fetch` | `oci+fetch://` | Storage initializer pulls image layers | Air-gapped clusters, legacy runtimes |

The `oci+native://` approach avoids the modelcar sidecar overhead and leverages
the container runtime's image pull and caching directly.  No benchmark data is
available yet — do not assume a performance advantage without measuring.

## References

- KServe issue #4083: OCI storage harmonization roadmap
- [KEP-4639](https://github.com/kubernetes/enhancements/issues/4639): OCI Volume Source
