# vLLM Binary Cache Support

## Overview

MCV supports three vLLM cache formats:

1. **vLLM Triton Cache Format** (legacy) - Stores `triton_cache/` and
   `inductor_cache/` inside rank directories
2. **vLLM Binary Cache Format** (current default) - Stores compiled artifacts in
   prefix directories with embedded Triton kernels
3. **vLLM Mega AOT Artifact Format** (PyTorch 2.12+) - Uses
   `VLLM_USE_MEGA_AOT_ARTIFACT=true` for enhanced AOT serialization

**AOT Compile Support**: MCV **supports** the `VLLM_USE_AOT_COMPILE=1` workflow,
which creates a separate cache structure at
`torch_compile_cache/torch_aot_compile/{hash}/rank_{rank}_{dp_rank}/model`.
AOT compile caches store ahead-of-time compiled models as single binary files
rather than multiple compilation artifacts. During preflight checks, AOT cache
compatibility is validated primarily via the summary label, as the cache metadata
contains limited hardware information.

Standard cache formats share the structure:
`torch_compile_cache/{hash}/rank_{rank}_{dp_rank}/{prefix}/`

The key differences are **inside the prefix directory**:

- **Triton format** (legacy): Contains `triton_cache/` and `inductor_cache/`
  subdirectories with unpacked Triton kernels
- **Binary format** (default): Contains `cache_key_factors.json` and multiple
  `artifact_compile_range_*` files with embedded Triton kernels
- **Mega AOT format** (PyTorch 2.12+): Same structure as binary format, but uses
  enhanced AOT serialization (indicated by `VLLM_USE_MEGA_AOT_ARTIFACT: true` in
  `cache_key_factors.json`)

**Note**: The `VLLM_USE_AOT_COMPILE=1` workflow uses a different structure at
`torch_compile_cache/torch_aot_compile/{hash}/rank_{rank}_{dp_rank}/model` and is
fully supported by MCV via `detectAOTCompileCache()` and AOT-specific preflight validation.

This document describes the **vLLM Binary, Mega AOT Artifact, and AOT Compile Formats** and how
torch.compile caching works with MCV.

**Important**: This document covers compilation mode 3 (`VLLM_COMPILE`) which uses
`~/.cache/vllm/torch_compile_cache/`. There is a separate vLLM feature controlled
by `VLLM_USE_AOT_COMPILE=1` (enabled by default in PyTorch 2.12+) that creates an
additional cache at `torch_compile_cache/torch_aot_compile/` with a different
structure. MCV fully supports this AOT compile workflow through automatic detection
and compatibility validation using the cache summary label.

## Torch Compile Architecture

### How vLLM Uses torch.compile

When vLLM is configured with compilation mode 3 via
`--compilation-config '{"mode": 3}'` (not enabled by default), it uses PyTorch's
`torch.compile` with TorchInductor backend to optimize model execution:

```text
Model Code → torch.compile → TorchInductor → Triton/CUDA Kernels → GPU Execution
```

**First Run (Compilation)**:

1. vLLM traces the model with Dynamo
2. TorchInductor compiles the graph
3. Triton generates optimized GPU kernels → `/tmp/torchinductor_$USER`
4. vLLM saves artifacts using `standalone_compile().save(format="binary")`
5. **PyTorch bundles the Triton kernels into the artifacts**
6. Complete cache saved to `~/.cache/vllm/torch_compile_cache/`

**Subsequent Runs (Cache Hit)**:

1. vLLM loads artifacts from `~/.cache/vllm/torch_compile_cache/`
2. **PyTorch extracts embedded Triton kernels → `/tmp/torchinductor_$USER`**
3. Execution resumes using extracted kernels (~10-20s vs 3-5min compilation)

### Binary vs Mega AOT Serialization

Both binary and mega AOT formats bundle Triton kernels in the artifacts and use
the same directory structure. They only differ in how the artifact files are
serialized:

**Binary Serialization** (default):

- Uses PyTorch `standalone_compile().save(format="binary")`
- Environment: `VLLM_USE_MEGA_AOT_ARTIFACT=false` (default in PyTorch <2.12)
- Multiple `artifact_compile_range_*` files per prefix
- Typical size: ~11MB for Qwen3-0.6B model

**Mega AOT Serialization** (PyTorch 2.12+):

- Uses PyTorch `AOTCompiledArtifact.serialize()` with bundled autograd cache
- Environment: `VLLM_USE_MEGA_AOT_ARTIFACT=true` (default in PyTorch 2.12+)
- More portable across PyTorch versions
- Same multi-artifact structure as binary format
- Typical size: Similar to binary format

**Important**: From MCV's perspective, both formats are **structurally identical**
and use the same detection and packaging logic:

```text
~/.cache/vllm/torch_compile_cache/{hash}/rank_{rank}_{dp_rank}/{prefix}/
```

### VLLM_USE_AOT_COMPILE vs VLLM_USE_MEGA_AOT_ARTIFACT

These are **two different features** that are often confused:

**`VLLM_USE_AOT_COMPILE`** (directory structure change):
- Enabled by default in PyTorch 2.12+
- Creates cache at: `torch_compile_cache/torch_aot_compile/{hash}/rank_X_Y/model`
- Single `model` file (~6.5MB) instead of multiple artifacts
- **Fully supported by MCV** via `detectAOTCompileCache()` function

**`VLLM_USE_MEGA_AOT_ARTIFACT`** (serialization format):
- Enabled by default in PyTorch 2.12+
- Uses regular cache path: `torch_compile_cache/{hash}/rank_X_Y/{prefix}/`
- Enhanced AOT serialization for better portability
- **Supported by MCV** (same structure as binary format)

When both are enabled, vLLM creates **both** cache locations.

### The /tmp Cache Directory

During compilation and execution, PyTorch creates temporary files:

```text
/tmp/torchinductor_$USER/
├── triton/0/{hash}/
│   ├── triton_.cubin    # Compiled GPU binary (ELF)
│   ├── triton_.source   # Triton source code
│   ├── triton_.ttir     # Triton IR
│   └── triton_.ptx      # PTX assembly
├── o7/, dp/, .../       # Python kernel cache
└── aotautograd/         # AOT autograd cache
```

**Lifecycle**:

- **First run**: Created during compilation
- **Cache hit**: Extracted from embedded artifacts
- **Cleanup**: Cleared on reboot (tmpfs) or manual deletion
- **Recreation**: Automatic on every vLLM start

**Key Insight**: This directory is **NOT needed for cache portability**.
The Triton kernels are already embedded in the binary artifacts (verified by
finding 42 ELF headers in a 5.3MB artifact file).

**MCV does NOT capture `/tmp`** - kernels auto-extract at runtime (~2 seconds).

## Binary Cache Format

### Directory Structure

The binary cache uses a structured directory layout:

```text
torch_compile_cache/
└── {hash}/                           # 10-character cache hash
    └── rank_{rank}_{dp_rank}/        # Per-rank cache
        └── {prefix}/                 # Model component
            ├── cache_key_factors.json
            ├── vllm_compile_cache.py
            ├── computation_graph.py
            └── artifact_compile_range_{start}_{end}_subgraph_{i}
```

### Key Components

#### 1. Cache Hash Directory

The hash directory is a 10-character truncated SHA256 hash derived from:

- Code hash (SHA256 of forward code files)
- Configuration hash (hash of vLLM config)
- Compiler hash (Inductor compiler state)
- Environment hash (compilation-affecting env vars)

#### 2. Rank Directory

Format: `rank_{rank}_{dp_rank}`

- `{rank}`: Distributed training rank
- `{dp_rank}`: Data parallel rank
- Allows multiple ranks to maintain separate caches

#### 3. Prefix Directory

Common prefixes:

- `backbone`: Main model component (default)
- `eagle_head`: Speculative decoding draft model

#### 4. Cache Files

**cache_key_factors.json**: Metadata tracking cache key components

```json
{
  "code_hash": "<sha256-hash>",
  "compiler_hash": "<compiler-hash>",
  "config_hash": "<config-hash>",
  "env": {
    "VLLM_TARGET_DEVICE": "cuda",
    "VLLM_COMPILE_CACHE_SAVE_FORMAT": "binary",
    "VLLM_MAIN_CUDA_VERSION": "12.9",
    ...
  }
}
```

**vllm_compile_cache.py**: Python dict mapping compile ranges to artifact
handles

**computation_graph.py**: Readable FX graph source code (for debugging)

**artifact_compile_range_* files**: Compiled artifacts

- **Binary format** (default): Single binary file per artifact
- **Unpacked format**: Directory containing Inductor output files

## Storage Formats

vLLM supports two storage formats for artifacts, controlled by
`VLLM_COMPILE_CACHE_SAVE_FORMAT`:

### Binary Format (default)

- **Env Var**: `VLLM_COMPILE_CACHE_SAVE_FORMAT=binary`
- **Artifacts**: Regular files
- **Multiprocess Safe**: Yes
- **Inspection**: Cannot easily inspect contents
- **Use Case**: Production deployments

```text
{prefix}/
├── artifact_compile_range_{start}_{end}_subgraph_0  (file, ~2.7 MB)
└── artifact_compile_range_{start}_{end}_subgraph_1  (file, ~2.1 MB)
```

### Unpacked Format

- **Env Var**: `VLLM_COMPILE_CACHE_SAVE_FORMAT=unpacked`
- **Artifacts**: Directories with Python/Triton files
- **Multiprocess Safe**: No (race conditions possible)
- **Inspection**: Can view and debug generated code
- **Use Case**: Development and debugging

```text
{prefix}/
├── artifact_compile_range_{start}_{end}_subgraph_0/  (directory)
│   ├── kernel_0.py
│   └── kernel_1.py
└── artifact_compile_range_{start}_{end}_subgraph_1/  (directory)
```

## MCV Metadata

### Container Image Labels

When a binary cache is packaged in a container image, MCV adds the
following labels:

```json
{
  "cache.vllm.image/entry-count": "1",
  "cache.vllm.image/cache-size-bytes": "35702329",
  "cache.vllm.image/format": "binary",
  "cache.vllm.image/summary": "{\"targets\":[...]}"
}
```

**Label Descriptions:**

- `entry-count`: Number of cache hash directories detected
- `cache-size-bytes`: Total size of the cache in bytes
- `format`: Storage format (`"binary"` or `"unpacked"`)
- `summary`: Hardware target information (JSON)

### Manifest Structure

The `manifest.json` file contains comprehensive metadata:

```json
{
  "vllm": [
    {
      "vllmHash": "{hash}",
      "cacheFormat": "binary",
      "binary": [
        {
          "rank": "rank_{rank}_{dp_rank}",
          "prefix": "{prefix}",
          "artifact_count": 17,
          "artifact_names": [
            "artifact_compile_range_{start}_{end}_subgraph_0",
            "artifact_compile_range_{start}_{end}_subgraph_1",
            ...
          ],
          "code_hash": "<sha256-hash>",
          "config_hash": "<config-hash>",
          "compiler_hash": "<compiler-hash>",
          "cache_save_format": "binary",
          "target_device": "cuda",
          "env": {
            "VLLM_TARGET_DEVICE": "cuda",
            "VLLM_COMPILE_CACHE_SAVE_FORMAT": "binary",
            "VLLM_MAIN_CUDA_VERSION": "12.9",
            ...
          }
        }
      ]
    }
  ]
}
```

**Manifest Fields:**

- `cacheFormat`: vLLM cache structure type (`"binary"` for new binary cache
  format, `"triton"` for legacy triton cache format)
- `binary[]`: Array of binary cache entries (one per rank/prefix combination)
- `cache_save_format`: Actual artifact storage format (`"binary"` or
  `"unpacked"`)
- `target_device`: Target hardware (`"cuda"`, `"rocm"`, `"tpu"`, `"cpu"`)
- `env`: Full environment variables from `cache_key_factors.json`

## Hardware Detection

MCV automatically detects hardware information from the system and combines it with cache metadata:

### CUDA

```json
{
  "backend": "cuda",
  "arch": "sm_75",
  "warp_size": 32,
  "ptx_version": 590,
  "cuda_version": "12.9"
}
```

- **Backend**: Extracted from `VLLM_TARGET_DEVICE` environment variable
- **Arch**: **Detected from actual GPU** on the system with "sm_" prefix (e.g., `sm_75` for Tesla T4, `sm_80` for A100, `sm_89` for RTX 4090). CUDA architectures are formatted with the "sm_" prefix to match NVIDIA conventions.
- **Warp Size**: 32 (CUDA default)
- **PTX Version**: PTX version from NVIDIA driver (e.g., 590 for driver 590.48.01)
- **CUDA Version**: CUDA toolkit version from `VLLM_MAIN_CUDA_VERSION` (e.g., "12.9")

**Important**: MCV detects the **actual GPU compute capability** from the system, not from environment variables. NVIDIA architectures are formatted as "sm_XX" (e.g., `sm_75` = sm_7.5 = Turing architecture) per `mcv/pkg/cache/vllm.go:buildBinaryCacheSummary()`. This ensures accurate compatibility checking between cached kernels and the target GPU.

### ROCm/HIP

```json
{
  "backend": "rocm",
  "arch": "gfx90a",
  "warp_size": 64
}
```

- **Backend**: Extracted from `VLLM_TARGET_DEVICE` environment variable
- **Arch**: **Detected from actual GPU** on the system (e.g., `gfx90a` for MI250, `gfx942` for MI300)
- **Warp Size**: 64 (AMD wavefront size)

**Note**: If GPU detection fails, MCV will warn that the cache may not be compatible with the current GPU. Always verify GPU compatibility before deployment.

## Format Detection

MCV automatically detects the vLLM cache format by inspecting the
filesystem:

1. **vLLM Binary Cache Detection**:
   - Looks for `rank_X_Y/` directories
   - Checks for `cache_key_factors.json`
   - Inspects `artifact_compile_range_*` entries
   - If entries are **files** → Binary artifact storage
   - If entries are **directories** → Unpacked artifact storage

2. **vLLM Triton Cache Detection** (fallback):
   - Looks for `triton_cache/` directory
   - Uses legacy vLLM triton cache extraction logic

This filesystem-based detection is more reliable than environment variables,
especially when caches are copied between systems.

### Format Indicators

MCV uses **three distinct format indicators** to describe vLLM caches. Each
serves a different purpose:

#### 1. Manifest `cacheFormat` (Cache Structure Type)

**Location**: `manifest.json` → `vllm[].cacheFormat`

**Values**: `"binary"` or `"triton"`

**Purpose**: Tells MCV extraction logic which vLLM cache structure to expect
inside rank directories

- `"binary"`: vLLM binary cache format - rank directories contain prefix
  subdirectories (e.g., `backbone/`)
- `"triton"`: vLLM triton cache format - rank directories contain
  `triton_cache/` subdirectory

**Example**:

```json
{
  "vllm": [{
    "cacheFormat": "binary",  // ← Extraction logic uses this
    "binary": [...]
  }]
}
```

This field determines which extraction code path MCV uses and is essential
for correctly unpacking the cache from the container image.

#### 2. Manifest `cache_save_format` (Artifact Storage Format)

**Location**: `manifest.json` → `vllm[].binary[].cache_save_format`

**Values**: `"binary"` or `"unpacked"`

**Purpose**: Records the actual artifact storage format detected from the
filesystem

- `"binary"`: Artifacts are individual files (multiprocess-safe, production
  use)
- `"unpacked"`: Artifacts are directories containing Python/Triton source
  files (debugging use)

**Example**:

```json
{
  "vllm": [{
    "cacheFormat": "binary",
    "binary": [{
      "rank": "rank_0_0",
      "prefix": "backbone",
      "cache_save_format": "binary",  // ← Detected from filesystem
      "artifact_count": 17,
      ...
    }]
  }]
}
```

This field is informational and helps users understand the internal artifact
format.

#### 3. Image Label `format` (User-Visible Format)

**Location**: OCI image labels → `cache.vllm.image/format`

**Values**: `"binary"` or `"unpacked"`

**Purpose**: Quick user-visible indicator of artifact storage format

- `"binary"`: For vLLM binary cache format with binary artifacts
- `"unpacked"`: For vLLM triton cache format OR vLLM binary cache format with
  unpacked artifacts

**Example**:

```json
{
  "cache.vllm.image/format": "binary",  // ← Quick indicator for users
  "cache.vllm.image/entry-count": "1",
  "cache.vllm.image/cache-size-bytes": "35702329"
}
```

This label allows users to quickly inspect cache format using `docker
inspect` or `skopeo inspect` without reading the full manifest.

### Format Mapping Table

| vLLM Format | Artifacts | `cacheFormat` | `cache_save_format` | Label |
| ----------- | --------- | ------------- | ------------------- | ----- |
| Binary | Binary files | `"binary"` | `"binary"` | `"binary"` |
| AOT | Binary files | `"binary"` | `"binary"` | `"binary"` |
| Triton | Unpacked dirs | `"triton"` | N/A | `"unpacked"` |

**Why Three Indicators?**

- **Manifest `cacheFormat`**: Extraction logic must know what's inside rank
  directories (`triton_cache/` subdirs vs `{prefix}/` subdirs)
- **Manifest `cache_save_format`**: Detailed metadata for debugging and
  compatibility checking
- **Image Label `format`**: Fast user-facing indicator without parsing full
  manifest

## Comparison: vLLM Binary Cache vs vLLM Triton Cache

| Aspect | Triton (Legacy) | Binary | Mega AOT |
| ------ | --------------- | ------------ | -------------- |
| **Structure** | `{hash}/rank_X_Y/` | `{hash}/rank_X_Y/` | `{hash}/rank_X_Y/` |
| **Inside Rank** | `triton_cache/` + `inductor_cache/` | `{prefix}/` | `{prefix}/` |
| **Metadata** | Triton JSON | `cache_key_factors.json` | `cache_key_factors.json` |
| **Serialization** | Unpacked | `standalone_compile().save()` | `AOTCompiledArtifact.serialize()` |
| **Storage** | Unpacked | Binary/unpacked | Binary |
| **Multiprocess** | No | Yes (binary) | Yes |
| **Distributed** | Full rank/DP | Full rank/DP | Full rank/DP |
| **Manifest** | `"triton"` | `"binary"` | `"binary"` |
| **Label** | `"unpacked"` | `"binary"`/`"unpacked"` | `"binary"` |
| **PyTorch Req** | Any | Any | 2.12+ |
| **Env Var** | - | `VLLM_USE_MEGA_AOT_ARTIFACT=0` | `VLLM_USE_MEGA_AOT_ARTIFACT=1` |
| **MCV Support** | ✅ Yes | ✅ Yes | ✅ Yes |

**Note**: The `VLLM_USE_AOT_COMPILE=1` workflow creates a cache at
`torch_compile_cache/torch_aot_compile/` and is fully supported by MCV through
`detectAOTCompileCache()` and preflight validation using the summary label.

## Complete Workflow Example

This section demonstrates the complete end-to-end workflow of capturing a vLLM cache, creating an OCI image, and extracting it on another system.

### Prerequisites

- Docker or Podman installed
- MCV binary built (`make mcv`)
- Access to a container registry (e.g., quay.io)
- GPU available on the system (NVIDIA or AMD)

### Step 1: Start vLLM Container

Start a vLLM container with a model. This example uses NVIDIA GPU with CUDA:

```bash
# For NVIDIA GPUs with CUDA 13.0
sudo podman run -d \
    --name vllm-server \
    --privileged \
    --device /dev/nvidia0:/dev/nvidia0 \
    --device /dev/nvidiactl:/dev/nvidiactl \
    --device /dev/nvidia-uvm:/dev/nvidia-uvm \
    --device /dev/nvidia-uvm-tools:/dev/nvidia-uvm-tools \
    -v /usr/lib64:/usr/lib64:ro \
    -v /usr/lib64:/usr/local/cuda-13.0/compat:ro \
    -v /usr/local/cuda:/usr/local/cuda:ro \
    -v ~/.cache/huggingface:/root/.cache/huggingface \
    --env 'LD_LIBRARY_PATH=/usr/lib64:/usr/local/cuda/lib64:/usr/local/cuda-13.0/compat' \
    -e NVIDIA_VISIBLE_DEVICES=all \
    -e NVIDIA_DRIVER_CAPABILITIES=compute,utility \
    -p 8000:8000 \
    --ipc=host \
    docker.io/vllm/vllm-openai:latest-cu130 \
    --model Qwen/Qwen3-0.6B
```

For AMD GPUs with ROCm, adjust the device mounts and environment variables accordingly.

### Step 2: Wait for Cache Generation

The vLLM server compiles kernels during model loading and warmup. Wait for the compilation to complete:

```bash
# Monitor vLLM logs to see compilation progress
sudo podman logs -f vllm-server

# Look for messages like:
# INFO 04-02 13:08:05 [monitor.py:48] torch.compile took 53.19 s in total
# INFO 04-02 13:08:28 [core.py:281] init engine (profile, create kv cache, warmup model) took 76.50 seconds
# INFO 04-02 13:08:31 [api_server.py:580] Starting vLLM server on http://0.0.0.0:8000

# Once you see "Starting vLLM server", the cache has been generated
```

The compiled kernels are stored in `/root/.cache/vllm/torch_compile_cache/` inside the container.

**Optional**: You can also send a test request to verify the server is working:

```bash
# Send a test request (cache already compiled during startup)
curl -s http://localhost:8000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "Qwen/Qwen3-0.6B",
    "messages": [{"role": "user", "content": "Hello!"}],
    "max_tokens": 50
  }' | jq -r '.choices[0].message.content'
```

### Step 3: Capture Cache from Container

Copy the generated cache from the running container to your host:

```bash
# Create directory for cache
mkdir -p ~/vllm-qwen-cache

# Copy cache from container
sudo podman cp vllm-server:/root/.cache/vllm ~/vllm-qwen-cache/

# Fix ownership
sudo chown -R $(whoami):$(whoami) ~/vllm-qwen-cache/

# Verify cache was captured
du -sh ~/vllm-qwen-cache/vllm
# Output: ~18M    /home/user/vllm-qwen-cache/vllm

# Inspect cache structure
ls -la ~/vllm-qwen-cache/vllm/torch_compile_cache/
# Should show hash directories (e.g., fe20897a43/)
```

### Step 4: Build Cache Image with MCV

Create an OCI container image containing the cache:

```bash
# Install buildah if not already installed
sudo dnf install -y buildah

# Build cache image
mcv -c \
    -i quay.io/myorg/vllm-qwen-cache:v1 \
    -d ~/vllm-qwen-cache/vllm \
    --builder buildah

# Output:
# INFO Using buildah to build the image
# INFO Detected cache components: [vllm]
# INFO Detected GPU: backend=cuda, arch=75, warpSize=32, PTX=590
# INFO Image built! 3cbede0b2cb5...
# INFO OCI image created successfully.
```

### Step 5: Inspect Cache Image

Verify the cache image metadata and labels:

```bash
# View image in buildah
buildah images | grep vllm-qwen-cache

# Inspect image labels
buildah inspect quay.io/myorg/vllm-qwen-cache:v1 | \
    jq -r '.OCIv1.config.Labels'

# Expected output:
# {
#   "cache.vllm.image/cache-size-bytes": "18152945",
#   "cache.vllm.image/entry-count": "2",
#   "cache.vllm.image/format": "binary",
#   "cache.vllm.image/summary": "{\"targets\":[{\"backend\":\"cuda\",\"arch\":\"75\",\"warp_size\":32,\"ptx_version\":590,\"cuda_version\":\"12.9\"}]}"
# }
```

**Important**: Notice that the `arch` field shows the **actual GPU compute capability** (e.g., `75` for Tesla T4 which is sm_7.5), not the CUDA toolkit version.

### Step 6: Push to Registry

Push the cache image to a container registry:

```bash
# Login to registry
buildah login quay.io

# Push image
buildah push quay.io/myorg/vllm-qwen-cache:v1

# Verify push
buildah images | grep vllm-qwen-cache
```

### Step 7: Extract Cache on Target System

On another system with compatible GPU, extract the cache:

```bash
# Pull and extract cache
mcv -e -i quay.io/myorg/vllm-qwen-cache:v1

# MCV performs preflight checks:
# 1. Fetches image and reads metadata
# 2. Detects local GPU (e.g., Tesla T4 with compute capability 75)
# 3. Compares with cache requirements
# 4. Extracts cache to ~/.cache/vllm/ if compatible

# Expected output on compatible GPU:
# INFO Preflight GPU compatibility check passed.
# INFO Preflight completed    matched="[0]" unmatched="[]"
# INFO Extracting cache to directory: /home/user/.cache/vllm
```

**Preflight Check Failure**: If the GPU is incompatible, MCV will reject the extraction:

```bash
# Example: Trying to use A100 (compute capability 80) cache on T4 (75)
mcv -e -i quay.io/myorg/vllm-a100-cache:v1

# Output:
# ERRO Preflight check failed: no compatible GPU found
# WARN No compatible GPUs found for the image.
```

### Step 8: Verify Cache with GPU Compatibility Check

Check compatibility without extracting:

```bash
# Check if current GPU is compatible with cached kernels
mcv --check-compat -i quay.io/myorg/vllm-qwen-cache:v1

# On compatible GPU (Tesla T4):
# No output means compatible

# On incompatible GPU:
# ERRO Preflight check failed: no compatible GPU found
# WARN No compatible GPUs found for the image.
```

### Step 9: View Detailed GPU Information

Get detailed information about system GPUs:

```bash
# Display GPU fleet information
mcv --gpu-info

# Output:
# INFO Detected 1 accelerator(s)
# GPU Fleet:
#   - GPU Type: TU104GL [Tesla T4]
#     Driver Version: 590.48.01
#     IDs: [0]
```

### Step 10: Use Cache with vLLM

Start vLLM with the extracted cache:

```bash
# The cache is now in ~/.cache/vllm/
# Start vLLM normally - it will automatically use the cache
podman run -d \
    --name vllm-with-cache \
    ... # same mounts and settings as before
    -v ~/.cache/vllm:/root/.cache/vllm \
    docker.io/vllm/vllm-openai:latest-cu130 \
    --model Qwen/Qwen3-0.6B

# vLLM will skip compilation and use cached kernels
# First request will be much faster!
```

### Workflow Summary

```text
┌─────────────────────────┐
│  1. Start vLLM          │
│     Container           │
└───────────┬─────────────┘
            │
            ▼
┌─────────────────────────┐
│  2. Wait for Kernel     │
│     Compilation         │
└───────────┬─────────────┘
            │
            ▼
┌─────────────────────────┐
│  3. Copy Cache from     │
│     Container to Host   │
└───────────┬─────────────┘
            │
            ▼
┌─────────────────────────┐
│  4. Build OCI Image     │
│     with MCV            │
└───────────┬─────────────┘
            │
            ▼
┌─────────────────────────┐
│  5. Push to Registry    │
└───────────┬─────────────┘
            │
            ▼
┌─────────────────────────┐
│  6. Pull & Extract on   │
│     Target System       │
│     (Preflight Checks)  │
└───────────┬─────────────┘
            │
            ▼
┌─────────────────────────┐
│  7. Use Cache with      │
│     vLLM on Target      │
└─────────────────────────┘
```

## Usage Examples

### Building a Cache Image

```bash
# Build from binary cache directory
mcv -c -d /path/to/model-binary-cache \
    -i quay.io/myorg/model-cache:v1 \
    --builder docker

# Result includes labels and manifest
```

### Extracting a Cache Image

```bash
# Extract cache from image
mcv -e -i quay.io/myorg/model-cache:v1

# MCV automatically detects format from manifest
# and extracts to appropriate location
```

### Inspecting Cache Metadata

```bash
# View image labels
skopeo inspect docker://quay.io/myorg/model-cache:v1 \
  | jq '.Labels'

# Expected output:
# {
#   "cache.vllm.image/format": "binary",
#   "cache.vllm.image/summary": "{\"targets\":[...]}",
#   ...
# }
```

## vLLM Source References

Key files in vLLM that implement binary cache:

- `vllm/envs.py:1512-1520` - `VLLM_COMPILE_CACHE_SAVE_FORMAT` definition
- `vllm/compilation/compiler_interface.py:186-327` -
  `InductorStandaloneAdaptor`
- `vllm/compilation/backends.py:245-346` - Compilation manager
- `vllm/compilation/backends.py:904-935` - `cache_key_factors.json` creation
- `vllm/compilation/backends.py:867-874` - Directory structure creation

## Best Practices

1. **Use binary format in production** for multiprocess safety
2. **Use unpacked format for debugging** to inspect generated code
3. **Include full env in manifest** for cache compatibility checking
4. **Verify hardware match** using image labels before deployment
5. **Check cache_save_format** in manifest when extracting caches
6. **Use AOT artifacts for cross-PyTorch-version portability** (requires PyTorch 2.12+)

## Migration from vLLM Triton Cache to vLLM Binary Cache

To migrate from vLLM triton cache format to vLLM binary cache format:

1. Update vLLM to a version that supports binary cache format
2. Set `VLLM_COMPILE_CACHE_SAVE_FORMAT=binary`
3. Run model warmup to generate new binary cache
4. Package new cache with MCV (automatically detected)
5. Both vLLM cache formats are supported, no breaking changes

## Practical Guide

### Generating a Cache

**Environment Setup**:

```bash
# For binary format (default):
export VLLM_COMPILE_CACHE_SAVE_FORMAT=binary
export VLLM_USE_MEGA_AOT_ARTIFACT=false  # or omit (default)

# For AOT format (more portable):
export VLLM_COMPILE_CACHE_SAVE_FORMAT=binary
export VLLM_USE_MEGA_AOT_ARTIFACT=true  # requires PyTorch 2.12+
```

**Run vLLM Warmup**:

```bash
# Enable compilation with mode 3 (VLLM_COMPILE) for cache generation
vllm serve my-model \
  --compilation-config '{"mode": 3}' \
  --tensor-parallel-size 1

# Alternatively, use the named mode:
# vllm serve my-model --compilation-config '{"mode": "VLLM_COMPILE"}' --tensor-parallel-size 1

# Make sample requests to trigger compilation:
curl http://localhost:8000/v1/completions \
  -H "Content-Type: application/json" \
  -d '{"model": "my-model", "prompt": "Hello", "max_tokens": 100}'
```

**Note**: Mode 3 (VLLM_COMPILE) is required for cache generation. Other modes:

- Mode 0: No compilation (default)
- Mode 1: Standard torch.compile
- Mode 2: Single Dynamo trace

**Verify Cache**:

```bash
ls -lh ~/.cache/vllm/torch_compile_cache/
# Should show a 10-char hash directory (e.g., 8d0a361fbc)

# Check cache contents:
find ~/.cache/vllm/torch_compile_cache/ -type f | head
```

### Packaging with MCV

**Create Container Image**:

```bash
mcv -c \
  -d ~/.cache/vllm/torch_compile_cache/{hash} \
  -i quay.io/myorg/my-model-cache:v1
```

**Verify Image Labels**:

```bash
skopeo inspect containers-storage:quay.io/myorg/my-model-cache:v1 \
  | jq '.Labels'

# Expected labels:
# {
#   "cache.vllm.image/cache-size-bytes": "95000000",
#   "cache.vllm.image/entry-count": "1",
#   "cache.vllm.image/format": "binary",
#   "cache.vllm.image/summary": "{\"targets\":[{\"backend\":\"cuda\",...}]}"
# }
```

### Using a Cached Image

**Extract Cache**:

```bash
mcv -e -i quay.io/myorg/my-model-cache:v1

# MCV extracts to: ~/.cache/vllm/torch_compile_cache/{hash}/
```

**Start vLLM**:

```bash
# vLLM automatically detects and uses the cache
vllm serve my-model --tensor-parallel-size 1

# Look for log message:
# INFO: Directly load the compiled graph(s) from the cache, took X.X s
```

### Cache Compatibility

A cache is compatible if:

1. **GPU architecture** matches (check: `nvidia-smi --query-gpu=compute_cap`)
2. **CUDA/ROCm version** compatible (check: `nvcc --version` or `rocm-smi`)
3. **PyTorch version** compatible
4. **Model code** unchanged (code hash must match)
5. **vLLM configuration** matches (TP size, compile level, etc.)
6. **Environment variables** match (see `cache_key_factors.json`)

**Check Compatibility**:

```bash
# View cache metadata:
cat ~/.cache/vllm/torch_compile_cache/*/rank_0_0/*/cache_key_factors.json \
  | jq '{target: .env.VLLM_TARGET_DEVICE, cuda: .env.VLLM_MAIN_CUDA_VERSION}'

# Compare with system:
nvidia-smi
# or
rocm-smi
```

## Troubleshooting

### Cache Not Being Used

**Symptom**: vLLM recompiles on every start despite having a cache

**Common Causes**:

See the [Cache Compatibility](#cache-compatibility) section above for requirements.

**Debug Steps**:

```bash
# 1. Check if cache exists
ls ~/.cache/vllm/torch_compile_cache/

# 2. Enable debug logging
export VLLM_LOGGING_LEVEL=DEBUG

# 3. Check for hash mismatch in logs
grep "cache" vllm.log | grep -i "hash\|miss"

# 4. Verify GPU compatibility
python -c "import torch; print(torch.cuda.get_device_capability())"
```

### Slow Startup with Cache

**Symptom**: vLLM takes 20+ seconds to start with cache

**Normal Behavior**: 10-20 seconds for kernel extraction from artifacts is expected

**If Slower**:

- Check disk I/O performance: `iostat -x 1`
- Verify `/tmp` is not on slow storage (NFS, etc.)
- Consider using `tmpfs` for `/tmp`: `df -h /tmp`

### Missing Kernels Error

**Symptom**: Runtime errors about missing Triton kernels

**Causes**:

1. Corrupted artifacts
2. Incomplete cache (warmup didn't cover all batch sizes)
3. Disk space issues during generation

**Solutions**:

```bash
# 1. Delete and regenerate cache
rm -rf ~/.cache/vllm/torch_compile_cache/*

# 2. Verify disk space
df -h ~/.cache/vllm/

# 3. Check artifact integrity
file ~/.cache/vllm/torch_compile_cache/*/rank_0_0/*/artifact_*
# Should show: "data" (binary format)
```

### AOT Artifact Serialization Issues

**Symptom**: AOT artifacts fail to load (when using `VLLM_USE_MEGA_AOT_ARTIFACT=true`)

**Requirements**:

- PyTorch 2.10.0 or later
- `VLLM_USE_MEGA_AOT_ARTIFACT=true`
- Compilation mode 3 (`--compilation-config '{"mode": 3}'`)

**Verify**:

```bash
# Check PyTorch version
python -c "import torch; print(torch.__version__)"

# Verify AOT flag in cache
grep "VLLM_USE_MEGA_AOT_ARTIFACT" \
  ~/.cache/vllm/torch_compile_cache/*/rank_0_0/*/cache_key_factors.json
```

## Advanced Topics

### Multi-GPU Caching

For tensor parallelism or pipeline parallelism:

```text
torch_compile_cache/{hash}/
├── rank_0_0/    # First tensor parallel rank
├── rank_0_1/    # Second tensor parallel rank
├── rank_1_0/    # First pipeline parallel rank
└── rank_1_1/    # Second pipeline + tensor parallel rank
```

MCV captures all rank directories. Extract the entire hash directory for
multi-GPU deployments.

### Multiple Model Components

Models with speculative decoding have multiple components:

```text
rank_0_0/
├── backbone/        # Main model
│   └── artifact_*
└── eagle_head/      # Draft model for speculation
    └── artifact_*
```

MCV captures all prefix directories automatically.

### Cache Size Optimization

**Typical Sizes**:

- Small models (< 1B params): 50-100 MB
- Medium models (1-10B params): 100-500 MB
- Large models (10B+ params): 500 MB - 2 GB

**Factors Affecting Size**:

- Number of compiled ranges (batch sizes)
- Number of layers
- Triton kernel count
- Autotune configurations

**Reduce Size**:

- Use fewer compile ranges: `VLLM_COMPILE_RANGES=[128,512]` vs default
- Binary format is smaller than unpacked
- AOT format is similar to binary

## See Also

- [spec-compat.md](./spec-compat.md) - OCI image specification
- [design.md](./design.md) - MCV architecture and design
- [vLLM Documentation](https://github.com/vllm-project/vllm) - vLLM project
