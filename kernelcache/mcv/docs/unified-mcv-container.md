# Unified MCV Container - NVIDIA + AMD Support

## Overview

The unified MCV container (`quay.io/gkm/mcv:unified`) includes **both NVIDIA (CUDA/NVML) and AMD (ROCm)** GPU support in a single image. It automatically detects which GPU vendor is available at runtime and uses the appropriate tooling.

## Quick Start

### Build the Unified Image

```bash
make build-image-mcv
# Tags as: quay.io/gkm/mcv:latest
```

### Basic Usage

```bash
# Auto-detects GPU vendor (NVIDIA or AMD)
podman run --rm --privileged \
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
- ROCm 6.2.4 provides `rocm-smi` and `amd-smi` CLI tools
- MCV checks for binary existence via `utils.HasApp()`
- Gracefully fails if tools not found (e.g., on NVIDIA nodes)

**Runtime Detection:**
```
On NVIDIA node: nvmlCheck() → ✓ uses NVML
On AMD node:    rocmCheck()  → ✓ uses rocm-smi
On CPU node:    both fail    → ✓ uses --no-gpu mode
```

## Container Variants Comparison

| Variant | Size | GPU Support | Use Case |
|---------|------|-------------|----------|
| `mcv:unified` | ~533 MB | NVIDIA + AMD | Mixed clusters, single image for all GPU nodes |
| `mcv:no-gpu` | ~174 MB | None | CI/CD, cache creation without GPU, arm64/mac |

## Usage Examples

### 1. Create Cache (No GPU Required)

```bash
# Works on any node - no GPU needed
podman run --rm --privileged \
  -v ~/.cache/vllm:/cache:ro \
  quay.io/gkm/mcv:unified \
  --create --image quay.io/myorg/llama3-cache:v1 \
  --dir /cache --no-gpu
```

**Why use unified here?** You can use the same image everywhere, simplifying CI/CD.

### 2. Extract with Auto GPU Detection

```bash
# On NVIDIA node - automatically uses NVML
podman run --rm --privileged \
  --device nvidia.com/gpu=all \
  -v ~/.cache/vllm:/cache \
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
podman run --rm --privileged \
  --device /dev/kfd --device /dev/dri \
  -v ~/.cache/vllm:/cache \
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
podman run --rm --privileged \
  --device nvidia.com/gpu=all \
  quay.io/gkm/mcv:unified \
  --check-compat --image quay.io/myorg/cache:v1
```

## Kubernetes Deployment

### Using in Kubernetes Jobs

The unified image works on any node type without requiring node selectors:

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: extract-cache
spec:
  template:
    spec:
      # No nodeSelector needed - works on any node!
      containers:
      - name: mcv
        image: quay.io/gkm/mcv:unified
        args:
          - "--extract"
          - "--image"
          - "quay.io/myorg/vllm-cache:v1"
          - "--dir"
          - "/cache"
        securityContext:
          privileged: true  # Access GPU device files
        volumeMounts:
        - name: cache
          mountPath: /cache
      volumes:
      - name: cache
        emptyDir: {}
      restartPolicy: Never
```

**Benefits:**
- ✅ Single Job definition works on any node type
- ✅ Simplifies deployment in mixed GPU clusters
- ✅ No need to maintain multiple Job definitions

### DaemonSet Deployment

For cache pre-extraction on all GPU nodes:

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
      # Runs on all GPU nodes (both NVIDIA and AMD)
      affinity:
        nodeAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
            nodeSelectorTerms:
            - matchExpressions:
              - key: nvidia.com/gpu.present
                operator: Exists
            - matchExpressions:
              - key: amd.com/gpu
                operator: Exists
      containers:
      - name: mcv
        image: quay.io/gkm/mcv:unified
        # Auto-detects GPU vendor on each node
        command: ["sh", "-c"]
        args:
          - |
            mcv --extract \
              --image quay.io/myorg/vllm-cache:v1 \
              --dir /kernel-caches/vllm
        securityContext:
          privileged: true
        volumeMounts:
        - name: kernel-caches
          mountPath: /kernel-caches
      volumes:
      - name: kernel-caches
        hostPath:
          path: /kernel-caches
          type: DirectoryOrCreate
```

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
docker run --rm \
  --device=/dev/kfd --device=/dev/dri \
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
          # Single image for all operations
          docker run --rm --privileged \
            -v $(pwd)/.cache:/cache:ro \
            quay.io/gkm/mcv:unified \
            --create --image quay.io/myorg/cache:${{ github.sha }} \
            --dir /cache --no-gpu

      - name: Push to registry
        run: docker push quay.io/myorg/cache:${{ github.sha }}
```

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
# Tags as: quay.io/gkm/mcv:latest

# Or with docker directly
docker build --platform linux/amd64 \
  --target mcv-unified \
  -t quay.io/gkm/mcv:unified \
  -f mcv/images/amd64.dockerfile \
  .
```

### Build the No-GPU Image (arm64/mac)

```bash
# From the repo root
make build-image-mcv-no-gpu
# Tags as: quay.io/gkm/mcv:latest-no-gpu

# Or with docker directly
docker build --platform linux/amd64 \
  --target mcv-minimal \
  -t quay.io/gkm/mcv:no-gpu \
  -f mcv/images/amd64.dockerfile \
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
- This is expected and harmless - container falls back to ROCm or no-GPU mode

### Issue: "couldn't find rocm-smi"

**On AMD nodes:**
- Check that ROCm drivers are installed on host: `rocm-smi --version`
- Verify devices exist: `ls -la /dev/kfd /dev/dri`
- Ensure container has device access: `--device=/dev/kfd --device=/dev/dri`

**On NVIDIA/CPU nodes:**
- This is expected and harmless - container falls back to NVML or no-GPU mode

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
- ROCm 6.2.4 (`amd-smi-lib`, `rocm-smi-lib`)
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
- [Kubernetes Deployment](./kubernetes-deployment.md) - K8s deployment patterns

## Summary

The unified MCV container simplifies deployment by providing a single image that works across all GPU types:

✅ **Single image** for NVIDIA, AMD, and CPU-only nodes
✅ **Auto-detection** of GPU vendor at runtime
✅ **Drop-in replacement** for existing MCV deployments
✅ **Simplified CI/CD** - one image for all environments
✅ **Mixed cluster support** - no node selectors required

**Build:**
```bash
make build-image-mcv        # GPU unified: quay.io/gkm/mcv:latest
make build-image-mcv-no-gpu # No-GPU:      quay.io/gkm/mcv:latest-no-gpu
```
