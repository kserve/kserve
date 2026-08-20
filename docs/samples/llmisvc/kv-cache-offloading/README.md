# KV Cache Offloading

KV cache offloading extends GPU memory by cascading evicted KV cache blocks to
cheaper tiers: GPU → CPU RAM → disk. This allows serving longer contexts or
more concurrent requests without increasing GPU count.

## Sizing the CPU tier

Read this before setting `kvCacheOffloading.cpu`. The value has two hard
dependencies that are not enforced by validation.

vLLM's offloading connector does not allocate the CPU tier on the heap. It mmaps
a file at `/dev/shm/vllm_offload_<engine-id>.mmap`, sizes it to the full `cpu`
value, and pre-faults every page during engine startup.

A negative size is rejected on apply. Anything else KServe passes straight
through, without adjusting it to a sane minimum, so a zero or an implausibly
small value reaches the connector exactly as written. Nothing catches those, and
the consequence only shows up when the engine starts and sizes its mapping from
that number.

### `/dev/shm` is sized for you

The presets size `/dev/shm` for NCCL traffic alone - 1Gi on single-node and
prefill/decode pods, 8Gi on both the leader and the workers of a data-parallel
deployment. Setting `cpu` grows that volume by the requested tier plus 20%, so a
10Gi tier on a single-node pod ends up with a 13Gi `/dev/shm`.

The 20% is the preset's defined headroom, not the controller's - each shipped preset carries it as
an annotation:

```yaml
metadata:
  annotations:
    internal.serving.kserve.io/kv-cache-shm-headroom-percent: "120"
```

Overriding the `dshm` volume still works if you want a larger ceiling, and the
tier is added on top of whatever you declare:

```yaml
spec:
  template:
    volumes:
      - name: dshm
        emptyDir:
          medium: Memory
          sizeLimit: 20Gi   # a 10Gi tier makes this 32Gi
```

A volume with no `sizeLimit` at all is left alone - an unbounded tmpfs already
accommodates any tier.

`sizeLimit` is a ceiling rather than a reservation: `tmpfs` allocates pages as they
are written, and the scheduler does not treat it as a resource request.

`cpu` sizes one engine's tier. A data-parallel pod runs `parallelism.dataLocal`
engines locally and the connector maps one file per engine, so such a pod can
need more than the automatic size - raise `sizeLimit` yourself and the tier is
still added on top of your value.

### Container memory must cover the tier

The offloaded cache lives on a memory-backed volume, so its populated pages are
charged to the container's memory cgroup. On a typical swap-disabled Kubernetes
node they cannot be reclaimed while in use, unlike ordinary file-backed page
cache - once written, the cache stays resident for the life of the container.

So the limit has to satisfy roughly:

```
limits.memory  >  model/process working set + kvCacheOffloading.cpu + runtime headroom
```

`requests.memory` should include the expected resident cache too, or the pod is
scheduled against a memory figure it will exceed in practice.

```yaml
kvCacheOffloading:
  cpu: "10Gi"           # <- resident in /dev/shm...
template:
  containers:
    - name: main
      resources:
        requests:
          memory: 42Gi   # <- ...and therefore accounted for here
        limits:
          memory: 74Gi
```

This is the one KServe does not size for you. The working set depends on the
model, which is nowhere in the spec, and an existing limit may be a hard
operational ceiling rather than an estimate - so adjusting it would mean guessing
at the part that is invisible and risking a pod that cannot be scheduled.

Applying a service whose limit cannot hold the tier at all - `limits.memory` no
larger than `cpu` - produces a warning naming both values. That check can prove a
contradiction, not adequacy: a 50Gi tier under a 64Gi limit is arithmetically
fine and still fails once the model's own footprint is counted. Only you, or
something that knows the model, can settle that.

The failure is quieter than the shared-memory one, too. Because the volume is
sized to fit, vLLM's startup free-space check passes, so an undersized limit does
not abort at startup - it surfaces as an OOMKill.

## How to choose a secondary disk tier

### I have a Ceph cluster (e.g. ODF on OpenShift)

Use **`pvc.ref`** with a pre-existing RWX PVC backed by CephFS. The PVC is shared
across all replicas, so cache built by one pod is available to others — useful
for multi-replica deployments.

```yaml
secondary:
  - fileSystem:
      pvc:
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

Use **`pvc.spec`**. The controller creates one ephemeral PVC per pod automatically.
The PVC is deleted when the pod is deleted.

```yaml
secondary:
  - fileSystem:
      pvc:
        spec:
          storageClassName: fast-local-nvme
          accessModes: [ReadWriteOnce]
          resources:
            requests:
              storage: 100Gi
```

> Because the PVC is pod-lifetime, the cache does not survive pod restarts.
> For a persistent cache, use `pvc.ref` with a pre-existing PVC instead.

## Mixing tiers

Multiple entries in `secondary` are allowed. vLLM consults them in order after
the CPU tier is full. You can mix backends freely:

```yaml
secondary:
  - fileSystem:
      pvc:
        ref:
          name: shared-cephfs-pvc   # tier 0: shared across replicas
  - fileSystem:
      emptyDir:
        size: "200Gi"               # tier 1: fast node-local spill
```
