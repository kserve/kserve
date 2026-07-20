# OCI fetch storage for InferenceService

This sample shows how to load a model from an OCI container image using the
`oci+fetch://` storageUri scheme (KServe issue [#4083](https://github.com/kserve/kserve/issues/4083)).

With `oci+fetch://`, the KServe **storage-initializer** pulls the image's layers
(using [oras-py](https://github.com/oras-project/oras-py)) and extracts the
`/models/` subtree into the shared `/mnt/models` volume. This is runtime-agnostic:
unlike `oci+native://` it does not require Kubernetes ImageVolume support, and
unlike the default `modelcar` mode it does not run a sidecar for the pod's
lifetime — the image is pulled once in an init container.

## Prerequisites

| Requirement | Notes |
|---|---|
| KServe | this branch or later, with `enableOciModelSupport: true` |
| Model image layout | model files under `/models/` in the image (modelcar convention) |
| Registry access | public images pull anonymously; private images need an `imagePullSecret` |

## OCI image layout convention

The image must store model files under `/models/` (the modelcar convention). The
handler pulls the image, extracts only the `/models/` subtree into `/mnt/models`,
and discards the rest of the container rootfs. An image without a `/models/`
directory fails the pull with a clear error.

Multi-arch images (an OCI image index / Docker manifest list, as produced by
`docker buildx`) are resolved to the platform manifest matching the init
container's architecture (`linux/<GOARCH>`); a tag-only target therefore pulls
the correct single-arch model layers.

## How to apply

1. Replace the `storageUri` in `inference_service.yaml` with your OCI model image
   reference. The image must contain model files under `/models/`.

2. (Private registries only) create a docker-registry secret in the
   `kserve-test` namespace and uncomment both the `Secret` block and the
   `predictor.imagePullSecrets` field in `inference_service.yaml`:
   ```bash
   kubectl create secret docker-registry oci-fetch-registry-creds \
     --docker-server=ghcr.io --docker-username=<user> \
     --docker-password=<token> -n kserve-test
   ```

3. Apply the manifest:
   ```bash
   kubectl apply -f inference_service.yaml
   ```

## How to verify

Check that the InferenceService is created:
```bash
kubectl get inferenceservice sklearn-oci-fetch -n kserve-test
```

The storage-initializer init container performs the pull. Confirm it completed
successfully:
```bash
kubectl get pod -n kserve-test -l serving.kserve.io/inferenceservice=sklearn-oci-fetch \
  -o jsonpath='{.items[0].status.initContainerStatuses[?(@.name=="storage-initializer")].state.terminated.exitCode}'
# -> 0
```

Inspect the init container logs to see the pull:
```bash
kubectl logs -n kserve-test \
  -l serving.kserve.io/inferenceservice=sklearn-oci-fetch -c storage-initializer
```

Confirm the model files landed in the shared mount:
```bash
kubectl exec -n kserve-test \
  $(kubectl get pod -n kserve-test \
      -l serving.kserve.io/inferenceservice=sklearn-oci-fetch \
      -o jsonpath='{.items[0].metadata.name}') \
  -c kserve-container -- ls /mnt/models
```

## How registry authentication works

When the predictor declares an `imagePullSecret`, the admission webhook projects
the first secret's `.dockerconfigjson` as a `config.json` into the init container
(volume `kserve-oci-fetch-docker-config`, mounted under `/mnt/oci-fetch-auth`)
and sets the `KSERVE_OCI_DOCKER_CONFIG` env var to its path. The Python handler
passes that path to oras-py as an explicit `config_path` — oras-py ignores
`DOCKER_CONFIG` and otherwise only reads `~/.docker/config.json`, so the explicit
path is robust regardless of `$HOME` or the container user (UID 1000).

If multiple `imagePullSecrets` are present, the first is used and a warning is
logged; combine credentials into a single `dockerconfigjson` secret if you need
more than one registry. With no secret, the pull is anonymous.

## Custom CA bundle (private TLS)

If a custom CA bundle is configured for the storage initializer (via
`caBundleConfigMapName` in `inferenceservice-config`), it is mounted into the
fetch init container and exported as `REQUESTS_CA_BUNDLE` so oras-py trusts a
private registry's TLS certificate — mirroring the S3 handler's CA bundle
handling.

## When to use this vs alternatives

| Mode | URI scheme | How it works | When to use |
|---|---|---|---|
| `fetch` | `oci+fetch://` | Storage initializer pulls image layers, extracts `/models/` | Clusters/runtimes without ImageVolume; avoid a long-lived sidecar |
| `native` | `oci+native://` or `oci://` (if default) | Kubernetes ImageVolume — no sidecar | K8s ≥ 1.33, runtime with ImageVolume support |
| `modelcar` | `oci://` (default) | Sidecar container sharing `/mnt/models` | Any K8s version, existing modelcar images |

No benchmark data is available yet — do not assume a performance advantage of one
mode over another without measuring.

## Current limitations

- `LLMInferenceService` does **not** yet route `ociModelMode: fetch`; it falls back
  to modelcar. Fetch support for LLMISVC is a planned follow-up.
- Legacy `kubernetes.io/dockercfg` secrets are not supported for private-registry
  auth; use `kubernetes.io/dockerconfigjson` instead.
- Mixing `oci+fetch://` with non-OCI storage URIs (S3/GCS/HTTP) in the same pod is
  not supported.
- Multi-secret merging for `imagePullSecrets` is not implemented; the first
  `imagePullSecret` is used (combine credentials into one `dockerconfigjson`
  secret if you need more than one registry).

Step 4 of the #4083 roadmap (ModelPack) is a planned follow-up that builds on this
fetch path.

## References

- KServe issue #4083: OCI storage harmonization roadmap
- [oras-py](https://github.com/oras-project/oras-py): OCI registry client used by the handler
