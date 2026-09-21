# MCV No-GPU Mode Usage Guide

## Overview

MCV now supports creating and extracting cache images **without GPU hardware** using the `--no-gpu` flag. This is useful for:

- CI/CD pipelines without GPU access
- Building cache images on development machines
- Containerized workflows where GPU passthrough isn't available
- Reducing image size when GPU validation isn't needed

## How It Works

When `--no-gpu` is used, MCV:
1. **Skips GPU hardware detection** via NVML/ROCm libraries
2. **Extracts GPU information from cache metadata** (stored in cache files by vLLM/Triton)
3. **Skips preflight compatibility checks** (no hardware to compare against)

The cache already contains all necessary GPU information:
- `VLLM_TARGET_DEVICE` (cuda/rocm)
- `VLLM_PAGED_ATTN_ARCH` (sm_75, gfx1100, etc.)
- `VLLM_MAIN_CUDA_VERSION` / `ROCM_VERSION`

## Container Images

### No-GPU Image (Recommended for --no-gpu)
- **Size:** ~176MB
- **Includes:** MCV binary, buildah, basic container tools
- **Excludes:** ROCm/CUDA libraries
- **Tags:** `quay.io/gkm/mcv:no-gpu`, `quay.io/gkm/mcv:latest` // TODO update later with kserve images

> **Note:** For MCV, `:latest` resolves to the no-gpu variant (lighter, safer default for cache
> creation without GPU hardware). This differs from the agent and gkm-extract images where
> `:latest` resolves to the unified (GPU-capable) variant. Use `quay.io/gkm/mcv:unified`
> explicitly for GPU environments.

```bash
# Build
make build-image-mcv-no-gpu
# or directly:
podman build --target mcv-minimal -t quay.io/gkm/mcv:no-gpu -f mcv/images/Containerfile .

# Use — --create builds the image into the container's local containers-storage;
# chain `buildah push` in the same run so it reaches the registry before --rm
# removes the container (and its store). Mount registry auth for buildah (the
# image runs as appuser, UID 1000, so mount it under that user's home).
podman run --rm \
  --userns=keep-id:uid=1000,gid=1000 \
  -v /path/to/cache:/cache:ro \
  -v ${HOME}/.config/containers/auth.json:/home/appuser/.config/containers/auth.json:ro \
  --entrypoint sh \
  quay.io/gkm/mcv:no-gpu \
  -c "/mcv --create --image quay.io/myorg/cache:v1 --dir /cache --no-gpu \
      && buildah push quay.io/myorg/cache:v1"
```

### Unified Image (For GPU validation - NVIDIA + AMD)
- **Size:** ~533MB
- **Includes:** MCV binary, buildah, CUDA runtime (NVML), ROCm libraries, amd-smi, rocm-smi
- **Use case:** Preflight checks with NVIDIA or AMD GPU hardware; auto-detects GPU vendor at runtime
- **Tags:** `quay.io/gkm/mcv:unified`

```bash
# Build
make build-image-mcv
# or directly:
podman build --target mcv-unified -t quay.io/gkm/mcv:unified -f mcv/images/Containerfile .

# Use with NVIDIA GPU (device nodes are world-accessible; no group needed)
podman run --rm --device nvidia.com/gpu=all \
  -v /path/to/cache:/cache:U quay.io/gkm/mcv:unified \
  --extract --image quay.io/myorg/cache:v1 --dir /cache

# Use with AMD GPU
podman run --rm --device /dev/kfd --device /dev/dri --group-add keep-groups \
  -v /path/to/cache:/cache:U quay.io/gkm/mcv:unified \
  --extract --image quay.io/myorg/cache:v1 --dir /cache
```

On hosts with group-restricted GPU devices, the documented device mappings can make
amd-smi or rocm-smi fail. Add a runtime-supported mapping, such as Podman
`--group-add keep-groups` with crun or explicit host device GIDs, and run GPU preflight
as appuser.

**Writable mounts (rootless).** Extraction writes into the mounted directory as the
image's non-root user, so a plain rootless bind mount can fail with `permission
denied`. Append `:U` to every writable mount (e.g. `-v /cache:/cache:U`) so Podman
chowns the mount to the container user; on SELinux-enforcing hosts also add `:Z`
(private to this container) or `:z` (shared) — for example `:U,Z`. Note that `:U`
recursively changes ownership of the host directory; if that is undesirable, use
`--userns=keep-id` instead to map your host UID into the container so existing
ownership works without a chown. Docker has no `:U` — use `--user $(id -u):$(id -g)`
there. Read-only mounts (`--create`) do not need `:U`.

## Usage Examples

### Creating Cache Images (No GPU Required)

```bash
# Local binary
mcv --create --image quay.io/myorg/vllm-cache:v1 \
    --dir ~/.cache/vllm/torch_compile_cache \
    --no-gpu

# In container (minimal image) — --create stores the image in the container's
# local containers-storage; chain `buildah push` so it reaches the registry
# before --rm removes the store. Mount registry auth under appuser's home.
podman run --rm \
  --userns=keep-id:uid=1000,gid=1000 \
  -v ~/.cache/vllm:/cache:ro \
  -v ${HOME}/.config/containers/auth.json:/home/appuser/.config/containers/auth.json:ro \
  --entrypoint sh \
  quay.io/gkm/mcv:no-gpu \
  -c "/mcv --create --image quay.io/myorg/vllm-cache:v1 --dir /cache --no-gpu \
      && buildah push quay.io/myorg/vllm-cache:v1"
```

### Extracting Cache (No GPU Required)

```bash
# Local binary
mcv --extract --image quay.io/myorg/vllm-cache:v1 \
    --dir ~/.cache/vllm \
    --no-gpu

# In container (minimal image)
podman run --rm \
  -v ~/.cache/vllm:/cache:U \
  quay.io/gkm/mcv:no-gpu \
  --extract --image quay.io/myorg/vllm-cache:v1 --dir /cache --no-gpu
```

### Extracting with Preflight Check (GPU Required)

```bash
# Local binary (requires ROCm/CUDA libraries)
mcv --extract --image quay.io/myorg/vllm-cache:v1 \
    --dir ~/.cache/vllm
# Automatically runs preflight check if GPU is detected

# In container (full image)
podman run --rm \
  --device /dev/kfd --device /dev/dri --group-add keep-groups \
  -v ~/.cache/vllm:/cache:U \
  quay.io/gkm/mcv:unified \
  --extract --image quay.io/myorg/vllm-cache:v1 --dir /cache
```

### Compatibility Check Only (GPU Required)

```bash
# Check if cache image is compatible with local GPU
mcv --check-compat --image quay.io/myorg/vllm-cache:v1

# In container
podman run --rm \
  --device /dev/kfd --device /dev/dri --group-add keep-groups \
  quay.io/gkm/mcv:unified \
  --check-compat --image quay.io/myorg/vllm-cache:v1
```

## CI/CD Pipeline Example

```yaml
# GitHub Actions / GitLab CI
build-cache-image:
  runs-on: ubuntu-latest
  steps:
    - name: Checkout code
      uses: actions/checkout@v4

    - name: Generate vLLM cache
      run: |
        # Run vLLM to generate cache
        python generate_cache.py

    - name: Build and push cache OCI image
      run: |
        # --create stores the image in the container's local containers-storage,
        # so push it in the same run — the store is gone once --rm removes the
        # container. Mount registry auth under appuser's home (image runs as UID 1000).
        podman run --rm \
          --userns=keep-id:uid=1000,gid=1000 \
          -v $(pwd)/.cache/vllm:/cache:ro \
          -v ${HOME}/.config/containers/auth.json:/home/appuser/.config/containers/auth.json:ro \
          --entrypoint sh \
          quay.io/gkm/mcv:no-gpu \
          -c "/mcv --create --image quay.io/myorg/vllm-cache:${{ github.sha }} \
                --dir /cache --no-gpu \
              && buildah push quay.io/myorg/vllm-cache:${{ github.sha }}"
```

## GPU Access Requirements

### When GPU Access Flags Are Required

**NVIDIA GPUs:** (device nodes are world-accessible — no group needed)
- Docker: `--gpus all` (or `--gpus device=0,1` for specific devices)
- Podman: `--device nvidia.com/gpu=all`
- **ONLY needed for**: GPU validation/preflight checks (extract without `--no-gpu`, compatibility checks)
- **NOT needed for**: Creating or extracting with `--no-gpu` flag

**AMD GPUs:** (`/dev/kfd` and `/dev/dri/*` are group-owned — the non-root user must join the render/video group)
- Podman (crun): `--device /dev/kfd --device /dev/dri --group-add keep-groups` (preserves the host user's render/video groups)
- Docker: `--device /dev/kfd --device /dev/dri`, plus a `--group-add <gid>` for each mapped device node's owning group (`keep-groups` is Podman/crun-only). Device paths and GIDs are host-specific — `/dev/dri/renderD128` and the `video` group may be absent, and `/dev/kfd` and the various `/dev/dri/*` nodes can each be owned by a different GID — so **derive a GID for each node that actually exists** and pass only the ones you found (never a bare or empty `--group-add`). For example:
  ```bash
  # Collect the distinct owning GIDs of the GPU nodes present on this host.
  gpu_gids=$(for d in /dev/kfd /dev/dri/renderD* /dev/dri/card*; do
    [ -e "$d" ] && stat -c '%g' "$d"
  done | sort -u)
  [ -n "$gpu_gids" ] || { echo "no GPU device nodes found" >&2; exit 1; }

  docker run --rm \
    --user "$(id -u):$(id -g)" \
    --device /dev/kfd --device /dev/dri \
    $(printf -- '--group-add %s ' $gpu_gids) \
    -v /path/to/cache:/cache \
    quay.io/gkm/mcv:unified \
    --extract --image quay.io/myorg/cache:v1 --dir /cache
  ```
  Docker has no `:U`; run as your host UID/GID with `--user "$(id -u):$(id -g)"`
  and ensure the host cache directory is writable by that UID/GID.
  Alternatively, require operators to supply the GIDs explicitly (e.g. `--group-add 44 --group-add 993`) when the device layout is known ahead of time.
- **ONLY needed for**: GPU validation/preflight checks (extract without `--no-gpu`, compatibility checks)
- **NOT needed for**: Creating or extracting with `--no-gpu` flag

### Examples

```bash
# No GPU access needed - works on any system
podman run --rm \
  -v /path/to/cache:/cache:ro \
  quay.io/gkm/mcv:no-gpu \
  --create --image quay.io/myorg/cache:v1 --dir /cache --no-gpu

# NVIDIA GPU access required for validation (world-accessible nodes; no group needed)
podman run --rm --device nvidia.com/gpu=all \
  -v /path/to/cache:/cache:U \
  quay.io/gkm/mcv:unified \
  --extract --image quay.io/myorg/cache:v1 --dir /cache

# AMD GPU access required for validation
podman run --rm \
  --device /dev/kfd --device /dev/dri --group-add keep-groups \
  -v /path/to/cache:/cache:U \
  quay.io/gkm/mcv:unified \
  --extract --image quay.io/myorg/cache:v1 --dir /cache
```

## When to Use Each Mode

| Use Case | Image | Flag | GPU Required | Docker GPU Flags |
|----------|-------|------|--------------|------------------|
| Create cache image | `no-gpu` | `--no-gpu` | ❌ No | None |
| Extract cache (no validation) | `no-gpu` | `--no-gpu` | ❌ No | None |
| Extract cache (AMD GPU validation) | `unified` | (none) | ✅ Yes (AMD) | `--device /dev/kfd --device /dev/dri` |
| Extract cache (NVIDIA GPU validation) | `unified` | (none) | ✅ Yes (NVIDIA) | `--gpus all` |
| Check compatibility (AMD) | `unified` | (none) | ✅ Yes (AMD) | `--device /dev/kfd --device /dev/dri` |
| Check compatibility (NVIDIA) | `unified` | (none) | ✅ Yes (NVIDIA) | `--gpus all` |
| CI/CD builds | `no-gpu` | `--no-gpu` | ❌ No | None |
| Production deployment (AMD) | `unified` | (none) | ✅ Yes (AMD) | `--device /dev/kfd --device /dev/dri` |
| Production deployment (NVIDIA) | `unified` | (none) | ✅ Yes (NVIDIA) | `--gpus all` |

## Limitations

When using `--no-gpu`:
- ⚠️ No preflight compatibility checks (assumes cache metadata is correct)
- ⚠️ Cannot detect mismatches between cache and actual hardware
- ⚠️ PTX version validation skipped for CUDA caches

**Recommendation:** Use `--no-gpu` for building/distribution, but validate with actual GPU hardware before production deployment.

## Troubleshooting

### Error: "Could not detect GPU on system"
This is expected with `--no-gpu`. MCV will fall back to cache metadata.

### Error: "accelerator is nil"
You're trying to run preflight checks without `--no-gpu` and without GPU libraries. Either:
1. Add `--no-gpu` flag, OR
2. Use the unified image with GPU device access

### Error: "no targets found in binary cache metadata"
The cache doesn't contain GPU metadata. This happens if:
- Cache was created on a non-GPU system without proper environment variables
- vLLM cache is corrupted or incomplete

Check that your cache was generated by vLLM/Triton on a GPU system.
