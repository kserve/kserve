# Download models from a private ModelScope-compatible repository

The storage initializer accepts `modelscope://owner/model[:revision]` URIs.
The repository address is configured separately through `MODELSCOPE_ENDPOINT`;
when unset, the ModelScope Hub SDK uses its default public endpoint.
The server must implement the ModelScope Hub APIs, including model metadata,
revision listing and file downloads. A generic HTTP file server is insufficient.
Any file URLs returned by the server must also be reachable from the initializer.

## Per-InferenceService configuration

Create a Secret in the same namespace as the InferenceService:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: modelscope-storage
  namespace: default
type: Opaque
stringData:
  MODELSCOPE_ENDPOINT: "https://models.internal.example"
  # Optional: omit this key for repositories allowing anonymous downloads.
  MODELSCOPE_API_TOKEN: "replace-with-your-token"
```

Reference it using the existing storage Secret annotation:

```yaml
apiVersion: serving.kserve.io/v1beta1
kind: InferenceService
metadata:
  name: sklearn-modelscope
  namespace: default
  annotations:
    serving.kserve.io/storageSecretName: modelscope-storage
spec:
  predictor:
    model:
      modelFormat:
        name: sklearn
      storageUri: "modelscope://my-team/sklearn-iris"
```

Replace the model ID and format with those of your repository. KServe injects
both keys into `storage-initializer` using `secretKeyRef`. An endpoint-only
Secret is supported. Alternatively, reference the Secret in the `secrets` list
of the predictor's ServiceAccount and set `spec.predictor.serviceAccountName`.
The storage Secret annotation takes precedence over the ServiceAccount Secrets.
These paths require a controller containing the endpoint Secret injection change.

Setting `spec.predictor.model.env` only configures the serving container; it
does not configure the ModelScope endpoint of the storage initializer.

## Shared initializer configuration

For a shared endpoint, the existing `ClusterStorageContainer` mechanism also
accepts environment variables in `spec.container.env`. Add the following entry
to the storage container used for `modelscope://` downloads, preserving its
image, other settings and supported URI formats:

```yaml
spec:
  container:
    env:
      - name: MODELSCOPE_ENDPOINT
        value: "https://models.internal.example"
```

This shared configuration path already works with an initializer whose
ModelScope Hub SDK reads `MODELSCOPE_ENDPOINT` (verified with `modelscope-hub`
0.1.4 and 0.2.0). Configure the endpoint in one place: the storage container's env patch
is merged after Secret injection and can override the Secret's value.

## Verify the injected configuration

After creating a new InferenceService pod, inspect its initializer env:

```sh
kubectl get pod -l serving.kserve.io/inferenceservice=sklearn-modelscope \
  -o jsonpath='{range .items[*].spec.initContainers[?(@.name=="storage-initializer")].env[*]}{.name}{"="}{.value}{.valueFrom.secretKeyRef}{"\n"}{end}'
kubectl logs -l serving.kserve.io/inferenceservice=sklearn-modelscope \
  -c storage-initializer
```

For HTTPS endpoints, the initializer must trust the server's certificate chain.
Successful environment injection alone does not establish repository API
compatibility or successful model download.
