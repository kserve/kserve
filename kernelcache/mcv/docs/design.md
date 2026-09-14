
# Model Cache Vault (MCV) - Design Document

## Overview

**MCV** (Model Cache Vault) is a utility for packaging, validating,
and extracting prebuilt Model caches using OCI container
images. It supports GPU hardware preflight checks, cache manifest
validation, and developer-friendly debugging workflows.

## Goals

- Package prebuilt Triton or vLLM caches into OCI-compatible container images.
- Validate kernel or model caches against the host GPU for compatibility.
- Support multiple container image formats, including Docker and OCI standard.
- Extract and validate cache contents via:
  - Kernel metadata summary (for fast checks)
  - Full manifest analysis (for detailed checks)

## Supported Image Formats

MCV **create** produces **compat** cache images: cache content in a standard
gzip tarball layer. MCV **extract** selects the code path from **layer media
type**, not manifest type (OCI vs Docker Schema 2).

| Layer media type | Typical builder | Extract support |
|------------------|-----------------|-----------------|
| `application/vnd.docker.image.rootfs.diff.tar.gzip` | Docker / MCV `-c` | Yes (compat) |
| `application/vnd.oci.image.layer.v1.tar+gzip` | Buildah / Podman / MCV `-c --buildah` | Yes (compat) |
| `application/cache.<type>.content.layer.v1+<type>` | External / legacy | Yes (fallback only) |

See [spec-compat.md](./spec-compat.md) for the full compat specification.

## Key Features

- **Triton Support**: Supports Triton cache packaging and extraction.
- **vLLM Support**: Supports vLLM model cache packaging and extraction.
- **Cache Summary Validation**: Checks image metadata labels before
  extraction.
- **Full Manifest Validation**: Parses and validates a detailed manifest
  after extraction.
- **GPU Compatibility Checks**: Verifies compatibility of kernel cache
  against local GPUs using backend, architecture, warp size, and optionally
  PTX versions.
- **Clean Extraction**: Extracted kernels are cleaned up if they are not
  compatible.
- **Temporary Directories**: All staging operations occur in `/tmp/.mcv/*`
  to prevent host pollution.

## Image Label Schema

Labels are set per cache type detected in the image. There are no generic
cross-type labels — each component writes its own label set.

### Triton Cache Labels

<!-- markdownlint-disable  MD013 -->
<!-- Temporarily disable MD013 - Line length to keep the table formatting  -->
| Label                                     | Description                           |
|-------------------------------------------|---------------------------------------|
| `cache.triton.image/entry-count`          | Number of Triton kernel cache entries |
| `cache.triton.image/cache-size-bytes`     | Cache directory size in bytes         |
| `cache.triton.image/summary`              | Summary of Triton-compatible targets  |

### vLLM Cache Labels

| Label                                     | Description                           |
|-------------------------------------------|---------------------------------------|
| `cache.vllm.image/entry-count`            | Number of vLLM model cache entries    |
| `cache.vllm.image/cache-size-bytes`       | Cache directory size in bytes         |
| `cache.vllm.image/summary`                | Summary of vLLM metadata              |
| `cache.vllm.image/format`                 | Cache format (e.g. `binary`, `unpacked`) |

> **Note**: Labels are only included for cache types detected in the image.

<!-- markdownlint-enable MD013 -->

## Workflow Summary

### Creating an Image

```bash
mcv -c -i quay.io/example/triton-kernel -d /path/to/.triton/cache
```

- Copies kernel cache into build context
- Writes manifest.json with entry metadata
- Builds Docker or OCI image using Docker or Buildah
- Labels image with summary + entry count

### Extracting and Validating

```bash
mcv -e -i quay.io/example/triton-kernel
```

- Downloads image
- Compares summary label to local GPU
- Extracts image if compatible
- Validates manifest
- Removes incompatible kernels if manifest fails

## Debugging & Logging

- Logging level configurable via `--log-level`
- Temporary cache lives under `/tmp/.mcv/`
- Kernel cache extracted under `~/.triton/cache/`

## Sequence Diagrams

### Image Creation

![Create](./images/create-puml.png)

### Image Extraction

![Extract](./images/extract-puml.png)
