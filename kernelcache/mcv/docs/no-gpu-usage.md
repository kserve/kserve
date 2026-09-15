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

### Minimal Image (DEFAULT - Recommended for --no-gpu)
**Size:** ~176MB
**Includes:** MCV binary, buildah, basic container tools
**Excludes:** ROCm/CUDA libraries
**Tags:** `quay.io/gkm/mcv:minimal`, `quay.io/gkm/mcv:latest`

```bash
# Build
podman build --target mcv-minimal -t quay.io/gkm/mcv:minimal -f mcv/images/amd64.dockerfile .

# Use
podman run --rm -v /path/to/cache:/cache quay.io/gkm/mcv:minimal \
  --create --image quay.io/myorg/cache:v1 --dir /cache --no-gpu
```

### AMD Image (For AMD GPU validation)
**Size:** ~923MB
**Includes:** MCV binary, buildah, ROCm libraries, amd-smi, rocm-smi
**Use case:** Preflight checks with AMD GPU hardware
**Tags:** `quay.io/gkm/mcv:amd`

```bash
# Build
podman build --target mcv-full -t quay.io/gkm/mcv:amd -f mcv/images/amd64.dockerfile .

# Use with AMD GPU device
podman run --rm --device /dev/kfd --device /dev/dri \
  -v /path/to/cache:/cache quay.io/gkm/mcv:amd \
  --extract --image quay.io/myorg/cache:v1
```

### NVIDIA Image (For NVIDIA GPU validation)
**Size:** ~356MB
**Includes:** MCV binary, buildah, CUDA runtime, NVML
**Use case:** Preflight checks with NVIDIA GPU hardware
**Tags:** `quay.io/gkm/mcv:nvidia`

```bash
# Build
podman build --target mcv-nvidia -t quay.io/gkm/mcv:nvidia -f mcv/images/amd64.dockerfile .

# Use with NVIDIA GPU
podman run --rm --device nvidia.com/gpu=all \
  -v /path/to/cache:/cache quay.io/gkm/mcv:nvidia \
  --extract --image quay.io/myorg/cache:v1 --dir /cache
```

## Usage Examples

### Creating Cache Images (No GPU Required)

```bash
# Local binary
mcv --create --image quay.io/myorg/vllm-cache:v1 \
    --dir ~/.cache/vllm/torch_compile_cache \
    --no-gpu

# In container (minimal image)
podman run --rm --privileged \
  -v ~/.cache/vllm:/cache:ro \
  quay.io/gkm/mcv:minimal \
  --create --image quay.io/myorg/vllm-cache:v1 --dir /cache --no-gpu
```

### Extracting Cache (No GPU Required)

```bash
# Local binary
mcv --extract --image quay.io/myorg/vllm-cache:v1 \
    --dir ~/.cache/vllm \
    --no-gpu

# In container (minimal image)
podman run --rm \
  -v ~/.cache/vllm:/cache \
  quay.io/gkm/mcv:minimal \
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
  --device /dev/kfd --device /dev/dri \
  -v ~/.cache/vllm:/cache \
  quay.io/gkm/mcv:amd \
  --extract --image quay.io/myorg/vllm-cache:v1 --dir /cache
```

### Compatibility Check Only (GPU Required)

```bash
# Check if cache image is compatible with local GPU
mcv --check-compat --image quay.io/myorg/vllm-cache:v1

# In container
podman run --rm \
  --device /dev/kfd --device /dev/dri \
  quay.io/gkm/mcv:amd \
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

    - name: Build cache OCI image
      run: |
        podman run --rm --privileged \
          -v $(pwd)/.cache/vllm:/cache:ro \
          quay.io/gkm/mcv:minimal \
          --create --image quay.io/myorg/vllm-cache:${{ github.sha }} \
          --dir /cache --no-gpu

    - name: Push cache image
      run: |
        podman push quay.io/myorg/vllm-cache:${{ github.sha }}
```

## GPU Access Requirements

### When GPU Access Flags Are Required

**NVIDIA GPUs:**
- Docker: `--gpus all` (or `--gpus device=0,1` for specific devices)
- Podman: `--device nvidia.com/gpu=all`
- **ONLY needed for**: GPU validation/preflight checks (extract without `--no-gpu`, compatibility checks)
- **NOT needed for**: Creating or extracting with `--no-gpu` flag

**AMD GPUs:**
- Docker & Podman: `--device /dev/kfd --device /dev/dri`
- **ONLY needed for**: GPU validation/preflight checks (extract without `--no-gpu`, compatibility checks)
- **NOT needed for**: Creating or extracting with `--no-gpu` flag

### Examples

```bash
# No GPU access needed - works on any system
podman run --rm \
  -v /path/to/cache:/cache:ro \
  quay.io/gkm/mcv:minimal \
  --create --image quay.io/myorg/cache:v1 --dir /cache --no-gpu

# NVIDIA GPU access required for validation
podman run --rm --device nvidia.com/gpu=all \
  -v /path/to/cache:/cache \
  quay.io/gkm/mcv:nvidia \
  --extract --image quay.io/myorg/cache:v1 --dir /cache

# AMD GPU access required for validation
podman run --rm \
  --device /dev/kfd --device /dev/dri \
  -v /path/to/cache:/cache \
  quay.io/gkm/mcv:amd \
  --extract --image quay.io/myorg/cache:v1 --dir /cache
```

## When to Use Each Mode

| Use Case | Image | Flag | GPU Required | Docker GPU Flags |
|----------|-------|------|--------------|------------------|
| Create cache image | minimal | `--no-gpu` | ❌ No | None |
| Extract cache (no validation) | minimal | `--no-gpu` | ❌ No | None |
| Extract cache (AMD GPU validation) | amd | (none) | ✅ Yes (AMD) | `--device /dev/kfd --device /dev/dri` |
| Extract cache (NVIDIA GPU validation) | nvidia | (none) | ✅ Yes (NVIDIA) | `--gpus all` |
| Check compatibility (AMD) | amd | (none) | ✅ Yes (AMD) | `--device /dev/kfd --device /dev/dri` |
| Check compatibility (NVIDIA) | nvidia | (none) | ✅ Yes (NVIDIA) | `--gpus all` |
| CI/CD builds | minimal | `--no-gpu` | ❌ No | None |
| Production deployment (AMD) | amd | (none) | ✅ Yes (AMD) | `--device /dev/kfd --device /dev/dri` |
| Production deployment (NVIDIA) | nvidia | (none) | ✅ Yes (NVIDIA) | `--gpus all` |

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
2. Use the full image with GPU device access

### Error: "no targets found in binary cache metadata"
The cache doesn't contain GPU metadata. This happens if:
- Cache was created on a non-GPU system without proper environment variables
- vLLM cache is corrupted or incomplete

Check that your cache was generated by vLLM/Triton on a GPU system.
