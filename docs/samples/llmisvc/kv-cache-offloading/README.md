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

The figure is the preset's, not the controller's - each shipped preset carries it as an annotation
giving the percentage of `cpu` to add, so 120 is the tier itself plus a fifth again:

```yaml
metadata:
  annotations:
    internal.serving.kserve.io/kv-cache-shm-percent-of-cpu: "120"
```

Declaring the `dshm` volume yourself turns the sizing off for that workload and
the value is used exactly as written. The controller cannot check its arithmetic
against the model you are actually running, so a number you wrote wins over one
it computed - including a smaller one:

```yaml
spec:
  template:
    volumes:
      - name: dshm
        emptyDir:
          medium: Memory
          sizeLimit: 20Gi   # used as-is; the tier is not added on top
```

That is per workload: declaring a size on `template` leaves `worker` and
`prefill` auto-sized. The controller logs which workloads it skipped and which
config declared the size.

This also applies when you customise a preset by copying it into your own
namespace, which is easy to miss. The copy is no longer a config KServe ships, so
the `sizeLimit` it inherited from the original counts as one you declared: sizing
stops, and the 1Gi the preset carries for NCCL is all you get.

The size is judged per volume, wherever the tier is configured, so moving
`kvCacheOffloading` onto the service does not bring the sizing back - the copied
`dshm` still carries a declared size. Either raise that `sizeLimit` in the copy to
cover the tier yourself, or keep referencing the shipped preset and put only your
delta in a config of your own, which leaves the preset owning `dshm`.

A volume with no `sizeLimit` at all is left alone - an unbounded tmpfs already
accommodates any tier.

### Already offloading before this landed

Preset names are versioned per release on installs that set
`LLM_INFERENCE_SERVICE_CONFIG_PREFIX`, and a service pins the name it was created
against. That is what keeps an upgrade from restarting anything - and it also
means an existing service keeps the preset it was created with, including the
older connector configuration. It will not pick up `CPUOffloadingSpec` on its own.

To move one, clear its pinned entry so the next reconcile resolves the current
preset:

```bash
kubectl patch llminferenceservice <name> --subresource=status --type=json \
  -p='[{"op":"remove","path":"/status/annotations/serving.kserve.io~1config-llm-template"}]'
```

Use the annotation key matching the preset the service resolves - check
`status.appliedConfigRefs`. This re-renders the workload, so that service restarts
once. Nothing else moves.

`sizeLimit` is a ceiling rather than a reservation: `tmpfs` allocates pages as they
are written, and the scheduler does not treat it as a resource request.

`cpu` sizes one engine's tier. A data-parallel pod runs `parallelism.dataLocal`
engines locally and the connector maps one file per engine, so such a pod needs
`dataLocal` times the automatic size - declare a `sizeLimit` that covers all of
them and it is used as written.

### Container memory must cover the tier

The offloaded cache lives on a memory-backed volume, so its populated pages are
charged to the container's memory cgroup. On a typical swap-disabled Kubernetes
node they cannot be reclaimed while in use, unlike ordinary file-backed page
cache - once written, the cache stays resident for the life of the container.

So the limit has to satisfy roughly:

```
limits.memory  >  model/process working set
                  + kvCacheOffloading.cpu x parallelism.dataLocal
                  + runtime headroom
```

`cpu` sizes one engine's tier, so a pod running several local engines is charged
one copy each.

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
