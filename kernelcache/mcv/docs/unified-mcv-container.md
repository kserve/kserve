# Unified MCV Container - NVIDIA + AMD Support

## Overview

The unified MCV container (`quay.io/gkm/mcv:unified`) includes **both NVIDIA (CUDA/NVML) and AMD (ROCm)** GPU support in a single image. It automatically detects which GPU vendor is available at runtime and uses the appropriate tooling.

## Quick Start

### Build the Unified Image

```bash
make build-image-mcv
# Tags as: quay.io/gkm/mcv:unified
```

### Basic Usage

```bash
# Auto-detects GPU vendor; pass vendor-specific device flags:

# NVIDIA
podman run --rm --device nvidia.com/gpu=all \
  -v /path/to/cache:/cache:Z,U \
  quay.io/gkm/mcv:unified \
  --extract --image quay.io/myorg/vllm-cache:v1 --dir /cache

# AMD — device nodes are group-owned, so the non-root user (UID 1000) must join
# the host render/video group. Podman: --group-add keep-groups (crun). Docker:
# pass numeric --group-add GIDs. See "Running GPU Preflight as Non-Root".
podman run --rm --device /dev/kfd --device /dev/dri \
  --group-add keep-groups \
  -v /path/to/cache:/cache:Z,U \
  quay.io/gkm/mcv:unified \
  --extract --image quay.io/myorg/vllm-cache:v1 --dir /cache
```

## How It Works

The unified container includes runtime libraries for both GPU vendors:

**NVIDIA Support:**
- CUDA 12.6.3 base provides `libnvidia-ml.so.1` (NVML)
- MCV uses `dlopen()` to load it dynamically at runtime
- Gracefully fails if library not found (e.g., on AMD nodes)

**AMD Support:**
- ROCm 7.0.1 provides `rocm-smi` and `amd-smi` CLI tools
- MCV checks for binary existence via `utils.HasApp()`
- Gracefully fails if tools not found (e.g., on NVIDIA nodes)

**Runtime Detection:**
```text
On NVIDIA node: nvmlCheck()  → ✓ uses NVML
On AMD node:    rocmCheck()  → ✓ uses rocm-smi
On CPU node:    both fail    → requires explicit --no-gpu flag
```

## Container Variants Comparison

| Variant | Size | GPU Support | Use Case |
|---------|------|-------------|----------|
| `mcv:unified` | ~533 MB | NVIDIA + AMD | Mixed clusters, single image for all GPU nodes |
| `mcv:no-gpu` | ~174 MB | None | CI/CD, cache creation without GPU, arm64/mac |

## Usage Examples

### 1. Create Cache (No GPU Required)

```bash
# Works on any node - no GPU needed.
# --create stores the OCI image in buildah's container-internal store;
# chain buildah push so the image reaches the registry before --rm removes the container.
podman run --rm \
  -v ~/.cache/vllm:/cache:Z,U \
  --entrypoint sh \
  quay.io/gkm/mcv:unified \
  -c "/mcv --create --image quay.io/myorg/llama3-cache:v1 --dir /cache --no-gpu \
      && buildah push quay.io/myorg/llama3-cache:v1"
```

**Why use unified here?** You can use the same image everywhere, simplifying CI/CD.

### 2. Extract with Auto GPU Detection

```bash
# On NVIDIA node - automatically uses NVML
podman run --rm \
  --device nvidia.com/gpu=all \
  -v ~/.cache/vllm:/cache:Z,U \
  quay.io/gkm/mcv:unified \
  --extract --image quay.io/myorg/cuda-cache:v1 --dir /cache

# Expected output:
# Initializing nvml Successful
# Using NVML to obtain GPU info
# Found 1 GPU devices
# Cache extracted successfully
```

```bash
# On AMD node - automatically uses rocm-smi
# --group-add keep-groups joins the host render/video group so the non-root user
# can open /dev/kfd and /dev/dri (Docker needs numeric --group-add GIDs instead).
podman run --rm \
  --device /dev/kfd --device /dev/dri \
  --group-add keep-groups \
  -v ~/.cache/vllm:/cache:Z,U \
  quay.io/gkm/mcv:unified \
  --extract --image quay.io/myorg/rocm-cache:v1 --dir /cache

# Expected output:
# Using ROCM to obtain GPU info
# Found 1 GPU devices
# Cache extracted successfully
```

### 3. Compatibility Check

```bash
# Automatically detects GPU and validates cache compatibility
podman run --rm \
  --device nvidia.com/gpu=all \
  quay.io/gkm/mcv:unified \
  --check-compat --image quay.io/myorg/cache:v1
```

## Running GPU Preflight as Non-Root

The MCV image runs as a non-root user (`appuser`, UID 1000) and no longer needs
`--privileged`. During extraction and `--check-compat`, MCV shells out to the
vendor tool (`amd-smi`/`rocm-smi`, `hl-smi`, or NVML) to read GPU info. Two
things must be true for that to work as a non-root user:

1. **The device nodes must be inside the container** — provide them with
   `--device` (or a CDI/device-plugin), the same as before.
2. **The user must be permitted to open them** — this depends on the device
   node's mode and group *on the host*.

Device-node permissions vary by vendor, which is why AMD needs an extra flag and
NVIDIA/Gaudi do not:

| Vendor | Typical device nodes | Typical mode/owner | Non-root needs a group? |
|--------|----------------------|--------------------|--------------------------|
| NVIDIA | `/dev/nvidia*` | `0666` (via nvidia-container-toolkit) | No |
| Intel Gaudi | `/dev/accel/accel*`, `/dev/accel/accel_controlD*` | `0666 root:render` | No |
| AMD | `/dev/kfd`, `/dev/dri/renderD*`, `/dev/dri/card*` | `0660 root:render` (+ `video`) | **Yes** |

> Device permissions are host-specific — confirm with `ls -l /dev/...` on your
> node. The GIDs are assigned by the host, so they cannot be baked into the
> image; supply them (or preserve host groups) at run time.

**NVIDIA** (nodes are world-accessible — device access only):

```bash
# Podman (CDI)
podman run --rm --device nvidia.com/gpu=all quay.io/gkm/mcv:unified \
  --check-compat --image quay.io/myorg/cache:v1
# Docker
docker run --rm --gpus all quay.io/gkm/mcv:unified \
  --check-compat --image quay.io/myorg/cache:v1
```

**Intel Gaudi** (nodes are `0666` — device access only, no group needed):

```bash
# Podman — --device accepts the whole /dev/accel directory.
podman run --rm --device /dev/accel quay.io/gkm/mcv:gaudi \
  --check-compat --image quay.io/myorg/cache:v1
```

To verify Gaudi device access from the container without a cache image, run
`--gpu-info` (queries `hl-smi` only — no cache dir or image build):

```bash
# Docker — --device does NOT accept a directory, so enumerate the nodes.
# seccomp/apparmor are unconfined because mcv calls buildah.InitReexec() at
# startup (for every subcommand), which sets up an unprivileged user namespace
# that Ubuntu 24.04's default container profile blocks.
docker run --rm \
  --user "$(id -u):$(id -g)" \
  --security-opt seccomp=unconfined \
  --security-opt apparmor=unconfined \
  $(printf -- '--device=%s ' /dev/accel/accel*) \
  quay.io/gkm/mcv:gaudi \
  --gpu-info
```

Expected output lists the detected fleet, e.g. `GPU Type: HL-325L` (gaudi3) with
one ID per device. The `newuidmap/newgidmap … Falling back to single mapping`
warnings from `InitReexec` are benign for `--gpu-info`.

> **Gaudi preflight is backend-only.** Habana Synapse recipe caches don't encode
> the target architecture or warp size in their recipe filenames, so MCV matches
> Gaudi caches on backend (`hpu`) alone — not on a specific Gaudi generation. A
> `gaudi2` cache will pass `--check-compat` on a `gaudi3` host and vice versa;
> validate generation compatibility out of band if it matters for your models.

**AMD** (render/DRI nodes are group-owned — add the group):

```bash
# Podman rootless (crun) — keep the host user's supplementary groups.
# Run this as a user that is a member of the render/video groups.
podman run --rm \
  --device /dev/kfd --device /dev/dri \
  --group-add keep-groups \
  quay.io/gkm/mcv:unified \
  --check-compat --image quay.io/myorg/cache:v1

# Docker — keep-groups is Podman/crun-only, so grant each GPU device node's
# owning GID explicitly. Device paths and GIDs are host-specific: renderD128
# and the "video" group may be absent, and /dev/kfd and the /dev/dri/* nodes
# can each be owned by a different GID. Derive a GID for every node that exists
# and pass only those (never a bare/empty --group-add).
gpu_gids=$(for d in /dev/kfd /dev/dri/renderD* /dev/dri/card*; do
  [ -e "$d" ] && stat -c '%g' "$d"
done | sort -u)
[ -n "$gpu_gids" ] || { echo "no GPU device nodes found" >&2; exit 1; }

docker run --rm \
  --device=/dev/kfd --device=/dev/dri \
  $(printf -- '--group-add %s ' $gpu_gids) \
  quay.io/gkm/mcv:unified \
  --check-compat --image quay.io/myorg/cache:v1
```

> `--group-add keep-groups` requires the `crun` runtime (the Podman default);
> it is a no-op under `runc`. For Docker, pass each device's GID with
> `--group-add` — derive them per-node as above, or require operators to supply
> the GIDs explicitly when the device layout is known ahead of time.

## Kubernetes Deployment

> **Note:** GKM operator deployments use the `gkm-extract` Job image
> (configured via `gkm.extract.image`) for in-cluster cache extraction — not
> MCV directly. The MCV image is for standalone cache packaging workflows
> (building OCI cache images outside the cluster). The examples below show MCV
> used standalone, independent of a GKM operator deployment.

### Using in Kubernetes Jobs

GPU extraction requires a GPU node and appropriate device access. For CPU-only
nodes, add `--no-gpu` to the args (or use `quay.io/gkm/mcv:no-gpu`).

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: extract-cache
spec:
  template:
    spec:
      # Pod-level securityContext. fsGroup chowns the PVC volume to GID 1000 so
      # the non-root user (UID 1000) can write the extracted cache and manifest
      # files. fsGroup and supplementalGroups are pod-level fields — they have no
      # effect if placed under a container's securityContext.
      securityContext:
        fsGroup: 1000
        # AMD only: /dev/kfd and /dev/dri/renderD* are group-owned
        # (root:render, mode 0660), so the non-root user must join that group.
        # NVIDIA and Gaudi device nodes are world-accessible (0666) and need none.
        # supplementalGroups: [<render-gid-on-node>]
      containers:
      - name: mcv
        image: quay.io/gkm/mcv:unified
        args:
          - "--extract"
          - "--image"
          - "quay.io/myorg/vllm-cache:v1"
          - "--dir"
          - "/cache"
        # Request the GPU through its device plugin instead of privileged: true.
        # The device plugin injects the device nodes and configures the device
        # cgroup, so no elevated privileges are needed and the pod is scheduled
        # onto a matching GPU node automatically.
        resources:
          limits:
            nvidia.com/gpu: 1   # AMD: amd.com/gpu: 1  |  Gaudi: habana.ai/gaudi: 1
        securityContext:
          runAsNonRoot: true
          runAsUser: 1000       # appuser (baked into the image)
          runAsGroup: 1000      # without this K8s defaults to GID 0
          allowPrivilegeEscalation: false
        volumeMounts:
        - name: cache
          mountPath: /cache
      volumes:
      - name: cache
        # emptyDir is lost when the pod completes; use a PVC so the extracted
        # cache persists for consuming workloads to read after the Job finishes.
        persistentVolumeClaim:
          claimName: gpu-cache-pvc  # must be created in advance
      restartPolicy: Never
```

**Benefits:**
- ✅ Runs unprivileged — GPU access comes from the device plugin, not `privileged: true`
- ✅ The same image works on any vendor; swap the `resources.limits` GPU key per node type
- ✅ CPU-only workflows supported with explicit `--no-gpu` (drop the GPU `resources.limits`)

### DaemonSet Deployment

For cache pre-extraction on all NVIDIA GPU nodes:

```yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: mcv-cache-preparer
spec:
  selector:
    matchLabels:
      app: mcv-cache-preparer
  template:
    metadata:
      labels:
        app: mcv-cache-preparer
    spec:
      # Runs on all nodes exposing the requested GPU resource.
      affinity:
        nodeAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
            nodeSelectorTerms:
            - matchExpressions:
              - key: nvidia.com/gpu.present
                operator: Exists
            # - matchExpressions:
            #  - key: amd.com/gpu
            #    operator: Exists
      # Pod-level securityContext. supplementalGroups is a pod-level field —
      # it is silently ignored under a container securityContext.
      securityContext:
        # AMD only — join the render group that owns /dev/kfd and /dev/dri/*
        # (NVIDIA/Gaudi device nodes are 0666 and need no supplemental group):
        # supplementalGroups: [<render-gid-on-node>]
      containers:
      - name: mcv
        image: quay.io/gkm/mcv:unified
        # Auto-detects GPU vendor on each node
        command: ["sh", "-c"]
        args:
          - |
            /mcv --extract \
              --image quay.io/myorg/vllm-cache:v1 \
              --dir /kernel-caches/vllm
        # GPU access via the device plugin — no privileged: true required.
        resources:
          limits:
            nvidia.com/gpu: 1   # AMD: amd.com/gpu: 1  |  Gaudi: habana.ai/gaudi: 1
        securityContext:
          runAsNonRoot: true
          runAsUser: 1000       # appuser in the image; Containerfile asserts id -u appuser = 1000
          runAsGroup: 1000      # without this K8s defaults to GID 0
          allowPrivilegeEscalation: false
        # The hostPath below must be writable by uid 1000 on each node.
        volumeMounts:
        - name: kernel-caches
          mountPath: /kernel-caches
      volumes:
      - name: kernel-caches
        # DirectoryOrCreate makes this root:root (0755); pre-create it owned by
        # UID/GID 1000 on each node, or use a PVC (see the note above) — fsGroup
        # cannot fix hostPath ownership.
        hostPath:
          path: /kernel-caches
          type: DirectoryOrCreate
```

> **Mixed-vendor clusters need one DaemonSet per vendor.** A DaemonSet applies one
> identical pod spec to every node it runs on, and a pod can request only one
> vendor's GPU resource. So even in a cluster that mixes NVIDIA, AMD, and Gaudi
> nodes, a single DaemonSet requesting (say) `nvidia.com/gpu` runs only on the
> NVIDIA nodes — the pods it tries to place on the AMD and Gaudi nodes stay
> unschedulable, since those nodes don't advertise that resource. To cover every
> vendor, deploy a separate DaemonSet for each, changing the `resources.limits`
> key (`amd.com/gpu` / `habana.ai/gaudi`), the `nodeAffinity` match key, and — for
> AMD only — adding `securityContext.supplementalGroups`. A single DaemonSet could
> only span all vendors by running `privileged: true` and mounting the host `/dev`
> directly, which is exactly what this unprivileged setup avoids.

## Testing

### Test on Different GPU Types

**NVIDIA GPU Test:**
```bash
# On a node with NVIDIA GPU
docker run --rm --gpus all \
  quay.io/gkm/mcv:unified \
  --extract --image quay.io/myorg/cuda-cache:v1 --dir /tmp/cache

# Should see: "Initializing nvml Successful"
```

**AMD GPU Test:**
```bash
# On a node with AMD GPU
# AMD device nodes are group-owned; grant their GIDs (keep-groups is Podman-only).
gpu_gids=$(for d in /dev/kfd /dev/dri/renderD* /dev/dri/card*; do
  [ -e "$d" ] && stat -c '%g' "$d"
done | sort -u)
docker run --rm \
  --device=/dev/kfd --device=/dev/dri \
  $(printf -- '--group-add %s ' $gpu_gids) \
  quay.io/gkm/mcv:unified \
  --extract --image quay.io/myorg/rocm-cache:v1 --dir /tmp/cache

# Should see: "Using ROCM to obtain GPU info"
```

**No-GPU Test:**
```bash
# On any node
docker run --rm \
  quay.io/gkm/mcv:unified \
  --extract --image quay.io/myorg/cache:v1 --dir /tmp/cache --no-gpu

# Should see: "GPU support disabled" and extraction succeeds
```

## CI/CD Integration

### GitHub Actions Example

```yaml
name: Build Cache Image

jobs:
  build-cache:
    runs-on: ubuntu-latest
    steps:
      - name: Build vLLM cache OCI image
        run: |
          # Buildah storage lives at /home/appuser/.local/share/containers inside
          # the image (owned by UID 1000). Mount a writable host directory there
          # so that --user 1000:1000 can write to it regardless of the runner UID.
          sudo install -d -o 1000 -g 1000 -m 700 /tmp/mcv-storage

          # --builder docker loads the image into the Docker daemon so the
          # subsequent docker push step can find it on the host.
          docker run --rm \
            --user 1000:1000 \
            --group-add "$(stat -c '%g' /var/run/docker.sock)" \
            --security-opt seccomp=unconfined \
            --security-opt apparmor=unconfined \
            -v $(pwd)/.cache:/cache:ro,Z \
            -v /tmp/mcv-storage:/home/appuser/.local/share/containers:Z \
            -v /var/run/docker.sock:/var/run/docker.sock \
            quay.io/gkm/mcv:unified \
            --create --image quay.io/myorg/cache:${{ github.sha }} \
            --dir /cache --no-gpu --builder docker

      - name: Push to registry
        run: docker push quay.io/myorg/cache:${{ github.sha }}
```

> **Why `--user 1000:1000` instead of `--user $(id -u):$(id -g)`?** Buildah's
> storage is pinned to `/home/appuser/.local/share/containers` inside the image,
> pre-created and owned by UID 1000 (`appuser`). Running as the host user's UID
> (typically 1001 on GitHub-hosted runners) would make those paths unwritable.
> Using a fixed `--user 1000:1000` keeps the UID consistent regardless of the
> runner's own UID. The `-v /tmp/mcv-storage:/home/appuser/.local/share/containers`
> mount gives UID 1000 a writable scratch area that does not outlive the job.
>
> **Why `--group-add`?** With `--builder docker`, MCV loads the built image into
> the Docker daemon over the mounted `/var/run/docker.sock`. A rootful socket is
> typically group-owned (`docker`) with mode `0660`. Because `--user 1000:1000`
> drops the container's supplementary groups, the process would lack access to the
> socket and the load would fail with `permission denied`. Adding
> `--group-add "$(stat -c '%g' /var/run/docker.sock)"` grants exactly that group,
> and is harmless for a rootless socket (which the user already owns).
>
> **Security note — rootful `/var/run/docker.sock`.** When the mounted socket
> connects to a rootful Docker daemon, `--group-add` gives MCV Docker API access.
> That access can start privileged containers and mount host paths; `--user` does
> not reduce these daemon permissions. Use a rootless or isolated daemon, or
> require trusted images and an isolated runner. Prefer a **rootless Podman**
> socket (`$XDG_RUNTIME_DIR/podman/podman.sock`) or a rootless Docker socket —
> neither requires `--group-add` and neither allows privilege escalation through
> the daemon.
>
> **Security note — `seccomp=unconfined` / `apparmor=unconfined`.** MCV runs
> buildah *inside* the container to assemble the OCI image, which needs
> `mount`/`unshare`/`pivot_root` and user-namespace operations that Docker's
> default seccomp and AppArmor profiles block for non-privileged containers.
> Disabling both is a deliberate, security-reviewed fallback — it is far narrower
> than `--privileged` (no added capabilities, host devices, or host namespaces),
> and the blast radius is bounded: the container runs as a non-root user
> (`--user`), performs a single packaging task, and is removed on exit (`--rm`).
> To harden the CI job further, replace these flags with scoped seccomp and
> AppArmor profiles that allow only buildah's required syscalls/operations, or run
> the same command under **rootless Podman**, which needs neither flag.

## Performance Comparison

### Startup Time by Variant

| Variant | Image Pull (first time) | Disk Usage | GPU Detection |
|---------|------------------------|------------|---------------|
| `mcv:unified` | ~15-20s | 533 MB | Auto (NVIDIA or AMD) |
| `mcv:no-gpu` | ~5-10s | 174 MB | None |

### When to Use Each Variant

**Use `mcv:unified` when:**
- ✅ You have a mixed GPU cluster (NVIDIA + AMD)
- ✅ You want a single image for all GPU environments
- ✅ You deploy across multiple clusters with different GPU types
- ✅ You want simplified deployment and maintenance

**Use `mcv:no-gpu` when:**
- ✅ Running on arm64 or mac development machines
- ✅ CI/CD pipelines without GPU access
- ✅ Cache creation or extraction without GPU validation
- ✅ Minimizing image size and pull time is important

## Building from Source

### Build the Unified Image (GPU - NVIDIA + AMD)

```bash
# From the repo root
make build-image-mcv
# Tags as: quay.io/gkm/mcv:unified

# Or with docker directly
docker build --platform linux/amd64 \
  --target mcv-unified \
  -t quay.io/gkm/mcv:unified \
  -f mcv/images/Containerfile \
  .
```

### Build the No-GPU Image (arm64/mac)

```bash
# From the repo root
make build-image-mcv-no-gpu
# Tags as: quay.io/gkm/mcv:no-gpu (also :latest)

# Or with docker directly
docker build --platform linux/arm64 \
  --target mcv-minimal \
  -t quay.io/gkm/mcv:no-gpu \
  -f mcv/images/Containerfile \
  .
```

### Build Both Variants

```bash
make build-images      # unified (GPU) + all other GPU images
make build-images-no-gpu  # no-gpu variants
```

### Push to Registry

```bash
make push-images        # push GPU variants
make push-images-no-gpu # push no-GPU variants
```

## Troubleshooting

### Issue: "Error initializing nvml"

**On NVIDIA nodes:**
- Ensure NVIDIA drivers are installed on host
- Verify GPU is visible: `nvidia-smi`
- Check container has GPU access: `--gpus all` or `--device nvidia.com/gpu=all`

**On AMD/CPU nodes:**
- Expected on CPU nodes — add `--no-gpu` for extraction without GPU hardware
- On AMD nodes, ensure ROCm device access is configured

### Issue: "couldn't find rocm-smi"

**On AMD nodes:**
- Check that ROCm drivers are installed on host: `rocm-smi --version`
- Verify devices exist: `ls -la /dev/kfd /dev/dri`
- Ensure container has device access: `--device=/dev/kfd --device=/dev/dri`

**On NVIDIA/CPU nodes:**
- Expected on non-AMD nodes — use `--no-gpu` on CPU nodes, or NVML on NVIDIA nodes

### Issue: Container works but no GPU detected

Both detection methods failed. Use `--no-gpu` flag to skip GPU validation:

```bash
podman run --rm \
  quay.io/gkm/mcv:unified \
  --extract --image myimage --dir /cache --no-gpu
```

## Choosing Between Variants

Use `mcv:unified` on GPU nodes (auto-detects NVIDIA or AMD), and `mcv:no-gpu` everywhere else:

```yaml
# GPU node Job (NVIDIA or AMD) - auto-detected at runtime
image: quay.io/gkm/mcv:unified
# No vendor-specific nodeSelector needed

---
# CI/CD or arm64/mac - no GPU required
image: quay.io/gkm/mcv:no-gpu
args: ["--create", "--image", "...", "--no-gpu"]
```

## Technical Details

### Base Image
- NVIDIA CUDA 12.6.3 base (Ubuntu 24.04)
- Provides `libnvidia-ml.so.1` for NVML

### Added Components
- ROCm 7.0.1 (`amd-smi-lib`, `rocm-smi-lib`)
- Buildah and container tools
- MCV binary (compiled with MCV client library)

### Runtime Detection Logic

From `mcv/pkg/accelerator/devices/device.go`:
```go
func registerDevices(r *Registry) {
    if config.IsStubEnabled() {
        staticCheck(r)  // Load from cache file in stub mode
    } else {
        amdCheck(r)     // Checks for amd-smi via utils.HasApp("amd-smi")
        rocmCheck(r)    // Checks for rocm-smi CLI tool
        nvmlCheck(r)    // Checks for libnvidia-ml.so.1 library
        // All checks run sequentially (no short-circuit)
    }
}
```

**AMD/ROCm Exclusivity:** Handled in `addDeviceInterface()`:
- When AMD registers: unregisters ROCM
- When ROCM registers: skipped if AMD already registered

## References

- [MCV README](../README.md) - Full MCV documentation
- [No-GPU Usage Guide](./no-gpu-usage.md) - Using MCV without GPU hardware

## Summary

The unified MCV container simplifies deployment by providing a single image that works across all GPU types:

- ✅ **Single image** for NVIDIA and AMD GPU nodes; CPU-only requires explicit `--no-gpu`
- ✅ **Auto-detection** of GPU vendor at runtime
- ✅ **Drop-in replacement** for existing MCV deployments
- ✅ **Simplified CI/CD** - one image for all environments
- ✅ **Mixed cluster support** - no node selectors required

**Build:**
```bash
make build-image-mcv        # GPU unified: quay.io/gkm/mcv:unified
make build-image-mcv-no-gpu # No-GPU:      quay.io/gkm/mcv:no-gpu (also :latest)
```
