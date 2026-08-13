# OCI Image Volume storage for InferenceService

This sample shows how to mount a model container image directly as a Kubernetes
[ImageVolume](https://kubernetes.io/docs/concepts/storage/volumes/#image) using
the `oci+native://` storageUri scheme (KServe issue [#4083](https://github.com/kserve/kserve/issues/4083)).

## Prerequisites

| Requirement | Minimum version |
|---|---|
| Kubernetes | 1.35+ (ImageVolume beta defaults-on; full support including subPath) |
| Kubernetes (with manual gate, full support) | 1.33–1.34 + `--feature-gates=ImageVolume=true` on apiserver and kubelet |
| Kubernetes (with manual gate, subPath unavailable) | 1.31–1.32 + `--feature-gates=ImageVolume=true` — subPath on ImageVolume VolumeMounts is forbidden; KServe surfaces an `OciImageVolumeCompatible` advisory condition |
| Container runtime | containerd ≥ 2.0 or CRI-O ≥ 1.31 |
| KServe | this branch or later |

## OCI image layout convention

KServe mounts the image volume with `subPath: "models"`, which means the container
runtime exposes the `/models/` directory inside the image at the configured `modelPath`
(default `/mnt/models`).  This matches the modelcar OCI image layout convention where
model files are stored under `/models/` in the image.

> **K8s 1.31–1.32 note**: `subPath` on `ImageVolume` VolumeMounts is not supported in the
> 1.31–1.32 alpha.  KServe sets an advisory `OciImageVolumeCompatible=False` condition on
> the InferenceService when this combination is detected.  Upgrade to K8s 1.33+ for full
> `oci+native://` support.

## How to apply

1. Replace the `storageUri` in `inference_service.yaml` with your OCI model image reference.
   The image must contain model files under `/models/` (exposed at `/mnt/models` via `subPath: "models"`).

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

And a container mount with `subPath: "models"`:
```
    Mounts:
      /mnt/models from mnt-models (ro, subPath=models)
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
the container runtime's image pull and caching directly.

## Measured startup behavior

Single-node benchmark (kind, Kubernetes v1.36.1, containerd 2.3.1, KServe
built from master at dbf6cfe, single cloud VM — AWS m6i.2xlarge: 8 vCPU /
32 GB RAM, 1 TB gp3 EBS at default provisioning (3,000 IOPS / 125 MiB/s) —
with control plane, registry, MinIO, and the predictor pod colocated, so
numbers reflect the delivery architecture, not WAN or multi-node effects;
the 140 GB tier is bound by that default disk throughput). Artifacts
of 2 / 14 / 140 GB (sized to fp16 weights of 1B / 7B / 70B-class models).
Median time from `InferenceService` apply to first successful prediction,
seconds:

| Path | 2 GB cold | 2 GB warm | 14 GB cold | 14 GB warm | 140 GB cold | 140 GB warm |
|---|---|---|---|---|---|---|
| `oci+native://` | 38.6 | **11.5** | 443.4 | **11.4** | 4,585.5 | **11.7** |
| modelcar (`oci://`) | 59.6 | 15.4 | 363.3 | 15.2 | 4,603.0 | 15.8 |
| `s3://` | 39.6 | 30.0 | 137.8 | 128.4 | 2,318.8 | 2,441.6 |

*Warm* = model image already in the node's containerd store; for `s3://`
there is no node-level cache, so every pod start re-downloads the artifact.

Takeaways:

- **Warm starts on OCI paths are size-independent.** With the image cached
  on the node, `oci+native://` reaches first prediction in ~12 s whether the
  model is 2 GB or 140 GB. Adding a replica of a 140 GB model on a warm node
  is ~11.7 s vs ~41 min of re-download over `s3://`.
- **The first cold pull costs more than a plain download** (~2× at 140 GB):
  containerd writes the layer blob and then unpacks it into a snapshot — two
  passes over the disk — where the storage initializer streams the object
  once. If a node serves a model more than once, the cached pulls quickly
  amortize this.
- modelcar shows the same pull behavior plus a constant ~4 s sidecar
  lifecycle overhead per pod.

Absolute numbers scale with disk/network throughput; the relative behavior
of the paths is the durable result. Full methodology, scripts and raw data:
<https://github.com/kliukovkin/oci-model-delivery-bench> — the harness runs
on any cluster if you want numbers for your own environment.

## References

- KServe issue #4083: OCI storage harmonization roadmap
- [KEP-4639](https://github.com/kubernetes/enhancements/issues/4639): OCI Volume Source
