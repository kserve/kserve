# Multi-Cache-Path Capture Contract

Status: proposed. KernelCache is not released yet, so this contract changes the current single-path values in place. It adds no compatibility mode for the old layout.

## Goal

A capture with N cache paths behaves like a capture with one cache path, repeated per path, and produces one OCI image. Every declared directory is captured, and on reuse every directory is mounted back at its `containerPath`. The `cachePaths` list that the webhook sends is the source of truth. MCV packages exactly the cache roots it is given. It does not choose which roots exist or where they go in the image.

## Terms

- Cache path: one entry of `cachePaths` (`containerName`, `containerPath`, `ociPath`).
- OCI path: the top-level directory of a cache path in the image. It is one path segment chosen by the webhook. There is no fixed list of names.
- Known OCI path: `io.vllm.cache` or `io.triton.cache`. MCV adds cache metadata for these. Any other name is packaged as plain content.
- Workspace: the directory passed to MCV. In the capture sidecar it is `/workspace/cache`.
- Cache root: an immediate child directory of the workspace. Its name is its OCI path.

## Path map

| Where | Path | Set by |
|---|---|---|
| Capture Pod, runtime container | `containerPath`, for example `/root/.cache/vllm` | cache path |
| Capture Pod, MCV sidecar | `/workspace/cache/<ociPath>` | webhook |
| OCI image | `<ociPath>/...`, relative to the image root | MCV, from the cache root name |
| Reuse Pod, image volume | `/var/run/gkm/kernel-cache/<ociPath>` | reuse webhook |
| Reuse Pod, runtime container | `containerPath`, symlinks into the image volume | reuse webhook |

The runtime container and the sidecar mount the same volume at different paths. `/workspace/cache` exists only inside the sidecar.

```text
runtime container                    sidecar                                image
/root/.cache/vllm    <- volume 0 ->  /workspace/cache/io.vllm.cache    ->  io.vllm.cache/
/root/.triton/cache  <- volume 1 ->  /workspace/cache/io.triton.cache  ->  io.triton.cache/
```

## Responsibilities

- Webhook: resolves cache paths, rejects duplicates, mounts one cache root per cache path, and passes the workspace to the sidecar.
- Sidecar entrypoint: checks that the cache roots match `cachePaths`, then runs MCV snapshot and create on the workspace.
- MCV: snapshots and packages every cache root in the workspace into one image.
- Controller and reuse webhook: no change. They keep using `cachePaths` and expect every declared OCI path to exist in the image.

## MCV contract

### Workspace input

`--workspace <dir>` selects workspace mode.

- `<dir>` is an absolute directory, not a symlink.
- Every immediate child is a directory, not a symlink. Any other entry fails the command.
- A child name that starts with `.wh.` fails the command, because it would be an OCI whiteout in the image.
- The workspace contains at least one cache root. There is no upper limit in MCV.
- `--workspace` is valid with `--snapshot` and with `--create --builder oci`. It is rejected with `--dir`, `--extract`, `--check-compat`, `--gpu-info`, and other builders.
- `MCV_CACHE_LINK_ROOT` applies to every cache root, as it does today.

`--dir` keeps its current behavior, including cache family auto-detection.

### Snapshot

```bash
mcv --snapshot --workspace /workspace/cache --snapshot-file /tmp/mcv/cache-snapshot.json
```

MCV records every cache root in one snapshot file in the current format (version 2). Each cache root is one `roots[]` entry whose `source` is the absolute cache root path. If any root fails, MCV writes no file.

```json
{
  "version": 2,
  "roots": [
    {
      "source": "/workspace/cache/io.triton.cache",
      "directories": [],
      "files": []
    },
    {
      "source": "/workspace/cache/io.vllm.cache",
      "directories": [],
      "files": []
    }
  ]
}
```

The snapshot format does not change. The OCI path is the base name of `source`. Existing snapshot fields, including recorded exclusions, keep their current behavior.

### Create

```bash
mcv --create --builder oci \
  --image quay.io/example/kernel-cache:capture-1 \
  --workspace /workspace/cache \
  --delta-from-snapshot \
  --snapshot-file /tmp/mcv/cache-snapshot.json \
  --result /tmp/mcv/result.json
```

With `--delta-from-snapshot`, the set of cache roots in the workspace must equal the set of roots in the snapshot. MCV decides whether a root changed with the current single-root rules.

| Root state | Decision |
|---|---|
| Legacy (version 1) snapshot | full |
| Empty baseline, still empty | unchanged |
| Empty baseline, content now | full |
| No difference | unchanged |
| Deleted file, or changed file directly below the root | full |
| Otherwise | delta, changed directories only |

Without `--delta-from-snapshot`, every root is full.

`Unchanged` is a capture result, not a per-root result. MCV returns `Unchanged` and pushes nothing only when every root is unchanged. Otherwise it builds one image and pushes it once. A changed root includes its selected delta, while an unchanged root is copied in full from the workspace. A root with a deletion or a root-level file change is also copied in full. An error in any root fails the command before the push. There is no partial image.

### Image layout

For each cache root, in name order:

- Full, delta, or unchanged: `<ociPath>/...` with the selected content. An unchanged root is full when another root changed.
- A root with no files (only directories or nothing): an empty `<ociPath>/` directory.

The image has one layer. Every declared OCI path exists in the image, so the reuse linker always finds its source directory.

A packaged root with a known OCI path also gets the cache metadata it gets today:

| OCI path | Family | Manifest | Labels |
|---|---|---|---|
| `io.vllm.cache` | vLLM | `io.vllm.manifest/manifest.json` | `cache.vllm.image/*`, `io.kserve.km/*` |
| `io.triton.cache` | Triton | `io.triton.manifest/manifest.json` | `cache.triton.image/*` |
| any other name | none | none | none |

For a known OCI path, MCV runs only that family's detector, on the staged copy as it does today. If the content is not detected as that family, the command fails, the same as `--dir` fails today when no cache type is detected. Roots with no files and roots with other names are not detected and get no metadata. A declared path the runtime never wrote to, such as a Triton cache on CPU, therefore does not fail the command.

Image labels are the union of the labels of the packaged roots. A key with two different values fails the command.

A workspace with one cache root with a known OCI path produces the same image as `--dir` on that root when auto-detection picks the same family.

### Result file

The format does not change.

```json
{
  "state": "Succeeded",
  "imageReference": "quay.io/example/kernel-cache@sha256:...",
  "cacheSizeBytes": 123456,
  "completedAt": "2026-11-12T12:00:00Z"
}
```

`cacheSizeBytes` is the sum over packaged roots. For `Unchanged`, `imageReference` is empty.

### Exit codes

Exit codes do not change: 3 for flag errors, 4 for snapshot errors, 1 for delta create errors, and 2 for full create errors.

## Sidecar entrypoint contract

- `MCV_CAPTURE_CONFIG.cacheDir` is the workspace, `/workspace/cache`. The other fields and `version: 1` do not change.
- Before the snapshot, the entrypoint lists the workspace. The set of child names must equal the set of `cachePaths[].ociPath`. A mismatch reports `CaptureFailed` without running MCV.
- Snapshot: `mcv --snapshot --workspace <cacheDir> --snapshot-file <file>`. The retry behavior does not change.
- Create: `mcv --create --builder oci --image <targetImage> --workspace <cacheDir> --result <file> --delta-from-snapshot --snapshot-file <file>`.

## Webhook contract

- The sidecar mounts the volume of `cachePaths[i]` at `/workspace/cache/<ociPath>`, using the resolved `ociPath`. Volume names (`mcv-cache-<i>`) and runtime container mounts do not change.
- `cacheDir` is `/workspace/cache`.
- `ociPath` is one path segment: no `/`, not `.` or `..`, and not starting with `.wh.`. An empty `ociPath` still resolves to `io.vllm.cache`. The current restriction to `io.vllm.cache` and `io.triton.cache` is removed.
- The webhook rejects cache paths that share a resolved `ociPath`, and cache paths of one container whose `containerPath` values are the same or nested. Without this check the Pod fails API validation on a duplicate mount path, or two roots read the same files. `captureconfig` validation applies the same rules, including the `ociPath` format.

## Unchanged components

- Controller: artifact `cachePaths` remain the declared cache paths.
- Reuse webhook: mounts the image at `/var/run/gkm/kernel-cache` and links `<ociPath>` to `containerPath` for each cache path.
- Snapshot file format and result file format.

## Out of scope

- Habana metadata. `io.habana.cache` is packaged as plain content in workspace mode.
- `--extract` and `--check-compat` on multi-root images. KServe reuse mounts the image directly and does not call them.
- Buildah and Docker builders in workspace mode.
- Resolving the `containerPath` of an explicit `io.triton.cache` entry from `TRITON_CACHE_DIR` (#6318). It does not depend on this contract. Adding a Triton cache path by default is PR 4.

## PR plan

Each PR builds and passes its tests on its own.

### 1. MCV: workspace input and multi-root snapshot

Adds `--workspace` for `--snapshot` and the workspace validation that later PRs reuse. `--create --workspace` is rejected until PR 2.

Tests:

- Three cache roots with any names write one snapshot with three roots sorted by source.
- A single-root workspace writes the same document as `--dir` on that root.
- A missing, relative, or symlinked workspace, an empty workspace, a file or symlink child, and a `.wh.` child name fail.
- `--workspace` with `--dir`, `--extract`, `--check-compat`, `--gpu-info`, or `--create` fails.

### 2. MCV: multi-root OCI image

Splits the per-root decision from staging and push, lets the build context hold several cache and manifest trees, and enables `--create --workspace`. `--dir` behavior and its tests do not change.

Tests:

- Both roots changed: one image with both cache trees, both manifests, and the union of labels.
- One root changed: its delta plus the other root copied in full, with manifests and labels for both known cache families.
- A root with another name: its content is packaged with no manifest and no labels, next to known roots.
- All roots unchanged: `Unchanged`, and nothing is pushed.
- Deletion in one root: that root is full; another independently changed root stays delta.
- Root content with a known OCI path not detected as that family: the command fails and nothing is pushed.
- A root with a known OCI path and no files: packaged as an empty directory with no manifest and no labels.
- Workspace roots differ from snapshot roots: the command fails.
- Without `--delta-from-snapshot`: every root is full.
- A single-root workspace produces the same image digest as `--dir`.
- `--create --workspace` without `--builder oci` fails.
- Conflicting label values fail the label merge.

### 3. KServe: wire the workspace

Changes the sidecar mount path and `cacheDir`, relaxes `ResolveOCIPath` to any single-segment name, adds duplicate validation to the webhook and `captureconfig`, switches the entrypoint to `--workspace` with the name check, and updates `capture-sidecar.md` and `oci-delta-capture.md`.

Tests:

- Sidecar mounts are `/workspace/cache/<ociPath>` and `cacheDir` is `/workspace/cache`, for one and two cache paths.
- Duplicate `ociPath` values, duplicate or nested container paths, and `ociPath` values that are not one segment are rejected with a clear message.
- The entrypoint runs the exact MCV arguments, and reports `CaptureFailed` on a name mismatch without running MCV.
- The existing single-path e2e keeps passing.

### 4. KServe: Triton cache path from `TRITON_CACHE_DIR`

When the runtime container sets `TRITON_CACHE_DIR`, the default cache paths add `{containerPath: $TRITON_CACHE_DIR, ociPath: io.triton.cache}` after the vLLM path. A `TRITON_CACHE_DIR` equal to or below the vLLM path adds nothing, since that directory is already captured. A `valueFrom` or empty value fails, the same as `VLLM_CACHE_ROOT`. Cache paths set on the `KernelCacheCapture` are used as they are.

Tests:

- Default cache paths with and without `TRITON_CACHE_DIR`, with it below the vLLM path, and with an invalid value.
- E2E: a workload with `TRITON_CACHE_DIR` is captured with two cache paths, the image holds `io.triton.cache/`, and the consumer links both paths. On the CPU e2e runtime Triton writes no files, so this checks the wiring, not Triton kernel reuse.
