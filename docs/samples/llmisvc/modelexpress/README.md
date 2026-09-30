# ModelExpress Examples

This directory contains example configurations that load model weights for an
`LLMInferenceService` through [ModelExpress](https://github.com/ai-dynamo/modelexpress) (MX).

## Overview

MX is a vLLM model loader. A replica loading a model first asks the MX server for a peer that
already holds the same weights, and pulls them GPU to GPU over RDMA. When no peer exists it
falls back to streaming the weights from object storage or from the MX server's cache. Every
replica that finishes loading becomes a source for the next one, so scaling out a model stops
paying for a full download per pod.

KServe's MX support is **experimental** and driven by `serving.kserve.io/exp-modelexpress-*`
annotations. It has two modes:

- **layered** keeps KServe's model delivery (storage initializer, PVC, OCI, LocalModelCache)
  and adds MX on top. Replicas still receive the model on disk; MX adds peer transfer.
- **native** hands weight loading to MX. KServe downloads only what vLLM needs besides the
  weights, and MX loads the weights from a peer, from object storage, or from its server cache.

## Prerequisites

- NVIDIA (CUDA) or Intel (XPU) accelerators. The MX loader does not run on CPU.
- A vLLM image with the `modelexpress` Python package installed. vLLM 0.23.0 and newer
  recognize `--load-format modelexpress`; older versions need `VLLM_PLUGINS=modelexpress`.
- A running MX server. [`setup-modelexpress.sh`](../../../../test/scripts/gh-actions/setup-modelexpress.sh)
  installs one with the upstream chart and the Kubernetes metadata backend.
- For peer transfer over RDMA: an RDMA device resource on the engine pods and the `IPC_LOCK`
  capability, as in the native `s3://` example. KServe adds neither.

## Examples

### 1. Native, S3 model ([llm-inference-service-modelexpress-native-s3.yaml](llm-inference-service-modelexpress-native-s3.yaml))

Weights stream from S3 into GPU memory with ModelStreamer and never land on the pod's disk.
The second replica loads from the first over RDMA. The model must be in safetensors format.

### 2. Native, Hugging Face model ([llm-inference-service-modelexpress-native-hf.yaml](llm-inference-service-modelexpress-native-hf.yaml))

The MX server downloads the model from the Hub once and streams it to each worker. Engine pods
need no Hub access and no `HF_TOKEN`; the server's credentials are used instead.

### 3. Layered ([llm-inference-service-modelexpress-layered.yaml](llm-inference-service-modelexpress-layered.yaml))

The model is mounted from a PVC as usual, and MX adds peer transfer. Layered mode accepts every
model URI scheme KServe supports.

## How It Works

### Annotations

| Annotation | Required | Purpose |
|---|---|---|
| `exp-modelexpress-mode` | yes | `native` or `layered`. Enables MX. |
| `exp-modelexpress-address` | yes | MX server as `host:port`, `http://host:port` or `https://host:port`. The client enables TLS for `https://` only. |
| `exp-modelexpress-token-audience` | no | Projects a ServiceAccount token with this audience for MX server authentication. |
| `exp-modelexpress-revision` | no | Overrides the revision in the P2P identity. Defaults to a hash of `spec.model.uri`. |

The admission webhook rejects an unknown mode, an address it cannot parse, empty values, and
the other annotations without a mode. When the service sets `spec.model.uri` itself, the webhook
also rejects model sources the mode cannot load; a URI that comes from `baseRefs` is checked by
the controller after merging and reported on `ModelExpressReady`.

### Weight delivery

```
native s3://                                 native hf://
                                             
storage initializer                          (no download)
  downloads everything except weights          |
  (*.safetensors, *.bin, *.pt, ...)            |
  to /mnt/models                               |
        |                                      |
vllm serve /mnt/models                       vllm serve <owner/model>
  --load-format modelexpress                   --load-format modelexpress
        |                                      |
MX: peer over RDMA?  -- yes --> done         MX: peer over RDMA?  -- yes --> done
        | no                                   | no
MX: ModelStreamer from MX_MODEL_URI          MX: server cache streams files
    (s3://...) into GPU memory                   into the Hugging Face cache
```

| Model URI | layered | native |
|---|---|---|
| `s3://` | Storage initializer downloads the model. MX adds peer transfer. | Storage initializer skips weight files. ModelStreamer streams the weights. S3 credentials and CA bundle are shared with the engine container. |
| `hf://` | Storage initializer downloads the model. MX adds peer transfer. | No download. vLLM gets the repo id and loads through the MX server cache. |
| `pvc://`, LocalModelCache | Mounted as usual. MX adds peer transfer. | Rejected. |
| `oci://`, `oci+native://` | Modelcar or image volume as usual. MX adds peer transfer. | Rejected. |

In native mode the base model never uses a LocalModelCache; LoRA adapters still can.

### What the controller adds to engine pods

Every engine pod (single node, prefill, and multi-node leader and worker) gets, on the `main`
container:

| Setting | Value |
|---|---|
| `--load-format` | `modelexpress`, unless the workload already sets a load format |
| `MX_SERVER_ADDRESS`, `MODEL_EXPRESS_URL` | The server address |
| `MX_MODEL_REVISION` | The revision annotation, or a hash of `spec.model.uri` |
| `POD_NAME`, `POD_NAMESPACE`, `POD_UID`, `MX_WORKER_HOST` | Downward API |
| `MX_AUTH_TOKEN_PATH` and a projected token volume | Only with a token audience |
| `MX_MODEL_URI` | Native `s3://` only |
| `MODEL_EXPRESS_NO_SHARED_STORAGE`, `MODEL_EXPRESS_CACHE_DIRECTORY`, `HF_HUB_OFFLINE` | Native `hf://` only |

Environment variables the workload already sets are kept. Single-node main and prefill pods
share the `<name>-kserve` ServiceAccount, so the MX server's allowlist can name one identity.

### The model argument

Native `hf://` has to pass the repo id to vLLM instead of `/mnt/models`. The vLLM presets run
`vllm serve ${KSERVE_MODEL_ARGS:-/mnt/models}` and declare `KSERVE_MODEL_ARGS` empty. The
controller fills it for native `hf://` and removes it otherwise, so every other service renders
`/mnt/models` as before. A custom preset opts in by declaring the same variable. Setting a
value for it in a service spec is rejected.

### Status

The `ModelExpressReady` condition reports whether MX is configured. It does not affect `Ready`.

| Situation | native | layered |
|---|---|---|
| Server resolved, model source supported | `True` | `True` |
| No usable server address (`ServerNotResolved`) | `False`; the workload is not rendered | `False`; the workload renders without MX |
| Unsupported model source or workload | `False`; the workload is not rendered | not applicable |

A stopped service ignores MX configuration, so stopping always works.

## Limitations

- CUDA and XPU only.
- ModelStreamer reads safetensors only; a native `s3://` model without `.safetensors` files
  fails to load.
- Native `s3://` accepts only `pvc://` LoRA adapters. The weight-file exclusion applies to every
  download the storage initializer makes, which would drop remote adapters' weights.
- Native `hf://` needs a preset that declares `KSERVE_MODEL_ARGS`. Services pinned to an older
  preset report `ModelExpressReady=False` with reason `UnsupportedWorkload`.

## Verification

```bash
# ModelExpressReady should be True
kubectl get llminferenceservice <name> -o jsonpath='{.status.conditions[?(@.type=="ModelExpressReady")]}'

# The engine pod should run with --load-format modelexpress
kubectl get deploy <name>-kserve -o jsonpath='{.spec.template.spec.containers[?(@.name=="main")].args}'

# Which strategy loaded the weights: the last "Trying strategy" without a matching "failed"
kubectl logs <engine-pod> -c main | grep -E "Trying strategy|Strategy .* failed"
```

The first replica of a native `s3://` service loads with `model_streamer`; replicas added later
load with `rdma`.

## Troubleshooting

### `ModelExpressReady=False` with reason `ServerNotResolved`

Set `serving.kserve.io/exp-modelexpress-address` to the MX server's gRPC endpoint.

### Every strategy fails and the engine exits with `No loading strategy succeeded`

For native `s3://`, check that the bucket holds `.safetensors` files and that the service's
ServiceAccount carries S3 credentials. The engine container receives the same credentials as the
storage initializer.

### The MX server pod fails with `container has runAsNonRoot and image will run as root`

The upstream chart sets `runAsNonRoot` while its image runs as root. Set
`securityContext.runAsUser` and `podSecurityContext.fsGroup`, as `setup-modelexpress.sh` does.

## Testing

`test/e2e/llmisvc/test_llm_modelexpress.py` has two tiers, selected by marker:

- `llmisvc_modelexpress and cluster_cpu` covers admission and the rendered engine pods. It runs
  in the LLMInferenceService CI workflow.
- `llmisvc_modelexpress and cluster_nvidia` serves a native `s3://` model and checks that a second
  replica loads from its peer over RDMA. It needs an MX-enabled vLLM image
  (`MODELEXPRESS_VLLM_CUDA_IMAGE`, see [`images/`](../../../../test/e2e/llmisvc/images)) and,
  for RDMA, `MODELEXPRESS_RDMA_RESOURCE`.

To run the GPU tier, install KServe with LLMInferenceService the way the CI workflow does, then
install an MX server and seed a safetensors model into the test bucket:

```bash
export MODELEXPRESS_ADDRESS="$(test/scripts/gh-actions/setup-modelexpress.sh)"
export MODELEXPRESS_MODEL_URI="$(test/scripts/gh-actions/seed-s3-model.sh Qwen/Qwen2.5-0.5B-Instruct)"
export MODELEXPRESS_VLLM_CUDA_IMAGE=<image> MODELEXPRESS_RDMA_RESOURCE=rdma/ib
test/scripts/gh-actions/run-e2e-tests.sh "llmisvc_modelexpress and cluster_nvidia" 1 envoy-gateway
```

The tests call the gateway's address, so run them from somewhere that can reach it, such as a pod
in the cluster.
