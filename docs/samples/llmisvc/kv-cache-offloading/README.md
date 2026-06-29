# KV Cache Offloading Examples

This directory contains example configurations for **tiered KV cache offloading** on an
`LLMInferenceService`. Offloading spills KV cache blocks out of GPU VRAM into cheaper
storage tiers, allowing longer sequences and higher concurrency without adding GPUs.

## Overview

vLLM's `OffloadingConnector` moves KV cache blocks between tiers transparently. KServe
exposes this via the `kvCacheOffloading` field on the `WorkloadSpec`, and the controller
translates it into the `--kv-transfer-config` argument for the vLLM serve command.

The tier hierarchy is:

```
GPU VRAM -> CPU RAM (primary, always required) -> secondary tiers (filesystem, object store)
```

When the CPU tier fills up, blocks are evicted to secondary tiers. vLLM's TieringManager
internally manages eviction and promotion across tiers. Multiple secondary tiers can be
configured — both `fileSystem` and `objectStore` accept arrays.

| Tier           | Minimum vLLM | When to use                                                   |
|----------------|--------------|---------------------------------------------------------------|
| CPU RAM        | 0.11         | Always required; simplest setup when host has spare RAM       |
| Filesystem     | 0.22         | Need more capacity than RAM alone; fast NVMe or shared PVC    |
| Object Store   | 0.23         | Very large caches, cross-node sharing via S3-compatible store |

> **Note:** `kvCacheOffloading` is only available in the `v1alpha2` API. Samples in this
> directory use `apiVersion: serving.kserve.io/v1alpha2`.

## Prerequisites

- KServe with `LLMInferenceService` CRD v1alpha2 installed
- A GPU node with sufficient host memory for the CPU tier
- For filesystem tiers: **vLLM 0.22 or later**; a volume mounted at the configured path
- For object store tiers: **vLLM 0.23 or later**; an S3-compatible object store reachable from the pod

## Examples

### 1. CPU-only offloading ([llm-inference-service-kv-cache-offloading-cpu.yaml](llm-inference-service-kv-cache-offloading-cpu.yaml))

The simplest configuration: spill KV cache blocks from GPU VRAM into host CPU RAM.
No secondary tiers — just omit `fileSystem` and `objectStore`.

**Use Case:**
- Host has spare RAM and you want longer sequences without a second GPU
- Good starting point before adding secondary tiers

**Deployment:**
```bash
kubectl apply -f llm-inference-service-kv-cache-offloading-cpu.yaml
```

**YAML snippet:**
```yaml
kvCacheOffloading:
  cpu:
    size: 10Gi
    evictionPolicy: lru
    blockSize: 64
```

### 2. Multi-tier offloading ([llm-inference-service-kv-cache-offloading-tiered.yaml](llm-inference-service-kv-cache-offloading-tiered.yaml))

Full tiered offloading: GPU -> CPU RAM -> filesystem and/or object store. You can use any
combination of filesystem and object store entries — the controller appends all entries into
vLLM's `secondary_tiers` array. vLLM's TieringManager internally manages eviction and
promotion across tiers.

**Use Case:**
- Long-context workloads that exceed available host RAM
- Cross-replica or cross-node KV cache sharing
- Combining fast local NVMe with large-capacity S3

**Deployment:**
```bash
kubectl apply -f llm-inference-service-kv-cache-offloading-tiered.yaml
```

**YAML snippet:**
```yaml
kvCacheOffloading:
  cpu:
    size: 10Gi
    evictionPolicy: lru
    blockSize: 64
  fileSystem:
    - path: /mnt/kv-cache-nvme
      readThreads: 16
      writeThreads: 16
  objectStore:
    - uri: "s3://s3.example.com/kv-cache/kv/"
      ioThreads: 8
```

#### Volume options for filesystem tiers

The filesystem tier requires a volume mounted at the configured `path`. The volume type
determines the sharing and persistence behavior:

| Volume type | Sharing between replicas | Persists across pod restarts | Notes |
|-------------|--------------------------|------------------------------|-------|
| `emptyDir`  | No (per-replica)         | No                           | Simplest; set `sizeLimit` to cap disk usage |
| `hostPath`  | Yes (same node only)     | Yes                          | Data is not cleaned up automatically |
| PVC (shared filesystem) | Yes (all replicas) | Yes                    | Best for cross-replica KV cache reuse |

The sample uses `emptyDir` with commented-out alternatives for `hostPath` and PVC.

## How It Works

The controller translates the `kvCacheOffloading` spec into vLLM's `--kv-transfer-config` JSON argument. The mapping:

| CRD field                      | vLLM JSON path                                           | Default          |
|--------------------------------|----------------------------------------------------------|------------------|
| (always)                       | `kv_connector` = `"OffloadingConnector"`                 | —                |
| (always)                       | `kv_role` = `"kv_both"`                                  | —                |
| `cpu.size`                     | `kv_connector_extra_config.cpu_bytes_to_use` (in bytes)  | (required)       |
| `cpu.evictionPolicy`           | `kv_connector_extra_config.eviction_policy`              | `lru`            |
| `cpu.blockSize`                | `kv_connector_extra_config.block_size`                   | GPU block size   |
| `fileSystem[].path`            | `secondary_tiers[N].root_dir`                            | (required)       |
| `fileSystem[].readThreads`     | `secondary_tiers[N].n_read_threads`                      | `16`             |
| `fileSystem[].writeThreads`    | `secondary_tiers[N].n_write_threads`                     | `16`             |
| `objectStore[].uri`            | Parsed into `store_config.endpoint`, `.bucket`, and `prefix` | (required)  |
| `objectStore[].storeConfig`    | Extra entries merged into `secondary_tiers[N].store_config`  | (optional)  |
| `objectStore[].ioThreads`      | `secondary_tiers[N].io_threads`                              | `4`         |

When any secondary tiers are present, the controller sets `kv_connector_extra_config.spec_name` = `"TieringOffloadingSpec"`.

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

The container template must mount a volume at the configured `fileSystem[].path`. Ensure the
volume exists and the mount is present in the pod spec. Check events with
`kubectl describe pod`.

### Object store connection failures

Verify that the `storeConfig` values are correct and the endpoint is reachable from the
pod network. Check pod logs for connection errors:
```bash
kubectl logs deployment/<name> -c main | grep -i "s3\|object.*store\|connection"
```

### vLLM reports "unknown kv_connector"

The vLLM version does not support `OffloadingConnector`. Upgrade to vLLM 0.11 or later
for CPU-only, 0.22 or later for filesystem tiers, or 0.23 or later for object store tiers.
