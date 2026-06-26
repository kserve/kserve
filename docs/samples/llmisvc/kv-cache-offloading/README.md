# KV Cache Offloading Examples

This directory contains example configurations for **tiered KV cache offloading** on an
`LLMInferenceService`. Offloading spills KV cache blocks out of GPU VRAM into cheaper
storage tiers, allowing longer sequences and higher concurrency without adding GPUs.

## Overview

vLLM's `OffloadingConnector` moves KV cache blocks between tiers transparently. KServe
exposes this via the `kvCacheOffloading` field on the `WorkloadSpec`, and the controller
translates it into the `--kv-transfer-config` argument for the vLLM serve command.

Three modes are supported:

| Mode  | Tiers                      | Minimum vLLM | When to use                                                   |
|-------|----------------------------|--------------|---------------------------------------------------------------|
| `cpu` | GPU VRAM -> CPU RAM        | 0.11         | Simplest setup; host has spare RAM                            |
| `fs`  | GPU -> CPU -> Filesystem   | 0.22         | Need more capacity than RAM alone; fast NVMe or shared PVC    |
| `obj` | GPU -> CPU -> Object Store | 0.23         | Very large caches, cross-node sharing via S3-compatible store |

All modes require a CPU tier as the first spill target; `fs` and `obj` add a secondary tier
behind it.

> **Note:** `kvCacheOffloading` is only available in the `v1alpha2` API. Samples in this
> directory use `apiVersion: serving.kserve.io/v1alpha2`.

## Prerequisites

- KServe with `LLMInferenceService` CRD v1alpha2 installed
- A GPU node with sufficient host memory for the CPU tier
- For `cpu` mode: support starts from **vLLM 0.11 or later**
- For `fs` mode: support starts from **vLLM 0.22 or later**; a PVC, `emptyDir`, or `hostPath` volume mounted at the configured path
- For `obj` mode: support starts from **vLLM 0.23 or later**; an S3-compatible object store reachable from the pod

## Examples

### 1. CPU-only offloading ([llm-inference-service-kv-cache-offloading-cpu.yaml](llm-inference-service-kv-cache-offloading-cpu.yaml))

The simplest configuration: spill KV cache blocks from GPU VRAM into host CPU RAM.

**Configuration:**
- Model: Qwen2.5-7B-Instruct, 1 replica
- CPU tier: 10 GiB with LRU eviction

**Use Case:**
- Host has spare RAM and you want longer sequences without a second GPU
- Good starting point before adding filesystem or object store tiers

**Deployment:**
```bash
kubectl apply -f llm-inference-service-kv-cache-offloading-cpu.yaml
```

**YAML snippet:**
```yaml
kvCacheOffloading:
  mode: cpu
  cpu:
    size: 10Gi
    evictionPolicy: lru
    blockSize: 64
```

### 2. Filesystem-tiered offloading ([llm-inference-service-kv-cache-offloading-fs.yaml](llm-inference-service-kv-cache-offloading-fs.yaml))

Two-tier offloading: GPU -> CPU RAM -> filesystem. Evicted blocks from the CPU tier are
written to disk, so the effective cache can be much larger than host RAM.

**Configuration:**
- Model: Qwen2.5-7B-Instruct, 1 replica
- CPU tier: 10 GiB with LRU eviction
- Filesystem tier: `/mnt/kv-cache`, 16 read threads, 16 write threads
- A PVC is mounted at the filesystem path

**Use Case:**
- Long-context workloads that exceed available host RAM
- Shared PVC for cross-pod KV cache reuse on the same node or across nodes

**Deployment:**
```bash
# Create the PVC first (adjust storage class and size for your cluster)
kubectl apply -f llm-inference-service-kv-cache-offloading-fs.yaml
```

**YAML snippet:**
```yaml
kvCacheOffloading:
  mode: fs
  cpu:
    size: 10Gi
    evictionPolicy: lru
    blockSize: 64
  fileSystem:
    path: /mnt/kv-cache
    readThreads: 16
    writeThreads: 16
template:
  containers:
    - name: main
      volumeMounts:
        - name: kv-cache-storage
          mountPath: /mnt/kv-cache
  volumes:
    - name: kv-cache-storage
      persistentVolumeClaim:
        claimName: kv-cache-pvc
```

> Replace `kv-cache-pvc` with the appropriate volume for your cluster.

### 3. Object-store-tiered offloading ([llm-inference-service-kv-cache-offloading-objectstore.yaml](llm-inference-service-kv-cache-offloading-objectstore.yaml))

Two-tier offloading: GPU -> CPU RAM -> S3-compatible object store. Useful when local
storage is limited or cross-node sharing without a shared filesystem is needed.

**Configuration:**
- Model: Qwen2.5-7B-Instruct, 1 replica
- CPU tier: 10 GiB with LRU eviction
- Object store: S3 endpoint with key prefix `kv/`, 8 I/O threads

**Use Case:**
- KV cache exceeds local disk capacity
- Cross-node cache sharing via a central object store
- Environments where S3 is readily available (cloud, MinIO, Ceph RGW)

**Deployment:**
```bash
# Replace storeConfig values with your actual S3 credentials and endpoint
kubectl apply -f llm-inference-service-kv-cache-offloading-objectstore.yaml
```

**YAML snippet:**
```yaml
kvCacheOffloading:
  mode: obj
  cpu:
    size: 10Gi
    evictionPolicy: lru
    blockSize: 64
  objectStore:
    storeConfig:
      endpoint: "s3.example.com"
      bucket: "kv-cache"
      access_key: <REPLACE_WITH_YOUR_ACCESS_KEY>
      secret_key: <REPLACE_WITH_YOUR_SECRET_KEY>
    prefix: "kv/"
    ioThreads: 8
```

> Replace `storeConfig` block with your actual object store credentials and endpoint.


## How It Works

The controller translates the `kvCacheOffloading` spec into vLLM's `--kv-transfer-config` JSON argument. The mapping:

| CRD field                    | vLLM JSON path                                           | Default          |
|------------------------------|----------------------------------------------------------|------------------|
| (always)                     | `kv_connector` = `"OffloadingConnector"`                 | —                |
| (always)                     | `kv_role` = `"kv_both"`                                  | —                |
| `cpu.size`                   | `kv_connector_extra_config.cpu_bytes_to_use` (in bytes)  | (required)       |
| `cpu.evictionPolicy`         | `kv_connector_extra_config.eviction_policy`              | `lru`            |
| `cpu.blockSize`              | `kv_connector_extra_config.block_size`                   | GPU block size   |
| `fileSystem.path`            | `secondary_tiers[0].root_dir`                            | (required for fs mode)|
| `fileSystem.readThreads`     | `secondary_tiers[0].n_read_threads`                      | `16`             |
| `fileSystem.writeThreads`    | `secondary_tiers[0].n_write_threads`                     | `16`             |
| `objectStore.storeConfig`    | `secondary_tiers[0].store_config`                        | (required for obj mode)|
| `objectStore.prefix`         | `secondary_tiers[0].prefix`                              | (none)           |
| `objectStore.ioThreads`      | `secondary_tiers[0].io_threads`                          | `4`              |

For `fs` and `obj` modes, the controller also sets `kv_connector_extra_config.spec_name` = `"TieringOffloadingSpec"` to activate the
secondary tier in vLLM.

## Tuning Tips

- **CPU tier size:** Do not exceed available host RAM. Oversizing causes OOMKill.
- **Eviction policy:** `lru` (least-recently-used) is the safe default. `arc` (adaptive
  replacement cache) can perform better for workloads with mixed access patterns but uses
  slightly more CPU overhead.
- **Block size:** Must be a multiple of the GPU block size. Defaults to the GPU block size when omitted.
- **I/O threads (fs):** Increase `readThreads`/`writeThreads` on fast NVMe storage.
  Decrease them on slower media to avoid saturating I/O bandwidth.
- **I/O threads (obj):** `ioThreads` controls concurrency to the object store. Increase
  for high-bandwidth connections; keep low for shared or throttled endpoints.

## Verification

```bash
# Confirm the LLMInferenceService is healthy
kubectl get llminferenceservice <name>

# Check that the generated vLLM args include --kv-transfer-config
kubectl get deployment <name> -o jsonpath='{.spec.template.spec.containers[0].args}' | grep kv-transfer-config
```

## Troubleshooting

### Pod OOMKilled after enabling CPU offloading

The `cpu.size` exceeds available host memory. Reduce the value or increase the node's
memory. Remember that `cpu.size` is in addition to the pod's regular memory request.

### Filesystem tier path not writable

The container template must mount a volume at the configured `fileSystem.path`. Ensure the
PVC exists, is bound, and the mount is present in the pod spec. Check events with
`kubectl describe pod`.

### Object store connection failures

Verify that the `storeConfig` values are correct and the endpoint is reachable from the
pod network. Check pod logs for connection errors:
```bash
kubectl logs deployment/<name> -c main | grep -i "s3\|object.*store\|connection"
```

### vLLM reports "unknown kv_connector"

The vLLM version does not support `OffloadingConnector`. Upgrade to vLLM 0.11 or later
for `cpu` mode, 0.22 or later for `fs` mode, or 0.23 or later for `obj` mode.
