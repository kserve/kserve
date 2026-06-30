# KV Cache Offloading

KV cache offloading extends GPU memory by cascading evicted KV cache blocks to
cheaper tiers: GPU → CPU RAM → disk. This allows serving longer contexts or
more concurrent requests without increasing GPU count.

## How to choose a secondary disk tier

### I have a Ceph cluster (e.g. ODF on OpenShift)

Use **`ref`** with a pre-existing RWX PVC backed by CephFS. The PVC is shared
across all replicas, so cache built by one pod is available to others — useful
for multi-replica deployments.

```yaml
secondary:
  - fileSystem:
      ref:
        name: my-cephfs-pvc   # provision this PVC with ocs-storagecluster-cephfs
        path: kv-cache/
```

### I have a single replica and don't need the cache to survive pod restarts

Use **`emptyDir`**. No StorageClass required; the node provides the disk. The
cache is lost when the pod is deleted or rescheduled, but there is zero
provisioning overhead.

```yaml
secondary:
  - fileSystem:
      emptyDir:
        size: "100Gi"
```

> The controller automatically adds an `ephemeral-storage` resource request
> equal to the `size` so the scheduler only places the pod on a node with
> sufficient local disk.

### I need a dedicated StorageClass (e.g. local NVMe) but don't want to manage PVCs myself

Use **`pvc`**. The controller creates one ephemeral PVC per pod automatically
using `ReadWriteOnce`. The PVC is deleted when the pod is deleted.

```yaml
secondary:
  - fileSystem:
      pvc:
        storageClassName: fast-local-nvme
        resources:
          requests:
            storage: 100Gi
```

> Because the PVC is pod-lifetime, the cache does not survive pod restarts.
> For a persistent cache, use `ref` instead.

## Mixing tiers

Multiple entries in `secondary` are allowed. vLLM consults them in order after
the CPU tier is full. You can mix backends freely:

```yaml
secondary:
  - fileSystem:
      ref:
        name: shared-cephfs-pvc   # tier 0: shared across replicas
  - fileSystem:
      emptyDir:
        size: "200Gi"             # tier 1: fast node-local spill
```
