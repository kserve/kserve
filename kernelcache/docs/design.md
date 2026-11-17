# KernelCache Design and Contracts

This document describes how KernelCache works today and the contracts between its components. It does not contain proposals or plans.

Rules for this document:

- A PR that changes a contract in [section 8](#8-contracts) updates this document in the same PR.
- For a large change, open a PR that only changes this document first, discuss it on the diff, and then implement against it.

MCV internals (snapshot format, builders, cache formats) are documented in [`kernelcache/mcv/docs`](../mcv/docs).

## 1. Overview

LLM serving runtimes such as vLLM compile graphs and kernels on first start (torch.compile, Triton) and write the results to a cache on disk. Starting again with the same model, runtime and options builds the same cache again.

KernelCache saves the cache written by the first pod (the producer) as an OCI image (the artifact). Later pods with the same conditions (consumers) get that artifact mounted, so the runtime cache path is already populated when the runtime starts.

KServe does not interpret cache contents. Its job is to bring back what the producer wrote, to the same path in a matching consumer, in a form that can be trusted. Producing and reading the cache is the runtime's job.

### 1.1 Design decisions

| Decision | Reason |
|---|---|
| Store the cache as an OCI image | Reuses registries, signing (cosign) and the node image cache |
| Reference artifacts by digest only | What is signed and verified is exactly what is used |
| Capture with a sidecar in the producer pod | Captures what the real workload wrote; no separate build Job |
| Store only the delta from a baseline | Files that ship in the runtime image are left out |
| Match by footprint | Hashes describe when a cache is valid |
| Link files into place on the consumer | Exposes read-only image content at the runtime path without copying |
| Pre-pull per node group | Removes image download time from consumer start |

## 2. Terms

| Term | Meaning |
|---|---|
| producer | Serving pod that creates a cache. Gets the MCV sidecar |
| consumer | Serving pod that reuses an existing cache |
| artifact | Result of a capture: a digest-pinned OCI image |
| cachePath | One cache directory: `containerName`, `containerPath`, `ociPath` |
| ociPath | Top-level directory name of that cache inside the artifact image |
| delivery | How the artifact gets to the consumer cache path. Selected by `mountType` |
| footprint | Hash used to match a cache to a pod |
| MCV | CLI that snapshots caches, builds and pushes OCI images, and extracts them (`kernelcache/mcv`) |

## 3. Components

```
                    +------------------------- control plane --------------------------+
  pod CREATE -----> | Pod mutating webhook --+-- cache found:    inject consumer         |
                    |                        +-- no cache found: inject MCV sidecar      |
                    | KCC status validating webhook                                    |
                    | Controllers: capture controller, capture reconciler,             |
                    |              cache reconciler, node reconciler                   |
                    +------------------------------------------------------------------+

  producer pod                        registry               each node
  +----------------------------+ push +--------+ pull +---------------------+
  | runtime        mcv sidecar | ---> | OCI    | <--- | prefetch Job        |
  | cache dir <-emptyDir->     |      | image  |      | node agent -> KCN   |
  +----------------------------+      +--------+      +---------------------+
                                          |
                                          | image volume
                                          v
                                  consumer pod
                                  +---------------------+
                                  | linker (init)       |
                                  | runtime             |
                                  +---------------------+
```

| Component | Responsibility |
|---|---|
| Pod mutating webhook | Injects the consumer mount or the MCV sidecar |
| KCC status validating webhook | Lets a sidecar write only `runtimeResult` of its own capture |
| Capture controller | Creates KCCs; manages per-capture reporter credentials (SA, Role, token) and push credentials |
| Capture reconciler | Turns `runtimeResult` into a phase, finalizes the artifact and identity, signs |
| Cache reconciler | Creates a KC from a completed KCC, picks the node group, verifies, runs prefetch Jobs, aggregates status |
| Node reconciler | Creates a KCN for each node in a node group, keeps the node agent DaemonSet placement in sync |
| Node agent | Writes per-cache preparation state of its node to the KCN |
| MCV sidecar (capture-entrypoint) | Baseline, readiness wait, MCV calls, result report |
| MCV CLI | Snapshot, delta, OCI image build and push, extract |
| Linker (init container) | Links cache files from the artifact into the runtime cache path |

The webhooks and controllers run in the localmodel controller manager. The node agent is a separate DaemonSet.

## 4. Resource model

| Resource | Scope | Spec written by | Status written by | Meaning |
|---|---|---|---|---|
| KernelCacheCapture (KCC) | namespaced | controller (a user can pre-create one with the same name to override) | controller, sidecar (`runtimeResult` only) | Capture for one workload revision |
| KernelCache (KC) | namespaced | controller (from a completed KCC) | controller | One reusable artifact and its rollout state |
| KernelCacheNode (KCN) | cluster | none | node agent | Per-cache preparation state of one node |
| KernelCacheNodeGroup (KCNG) | cluster | admin | none | Nodes where caches are prepared (`nodeSelector`, `tolerations`) |

```
InferenceService --(per revision)--> KCC --(complete)--> KC --(nodeGroupRef)--> KCNG
                                                          ^                       | per node
                                                          +--- KCN.cacheStatus <--+
consumer pod --(footprint match)--> KC
```

Names:

- KCC: `<isvc>-kcc-<pod-template-hash>`
- KC: `<first 38 chars of KCC name>-<first 12 chars of sha256(imageReference)>`
- Default artifact image: `<registry endpoint>/<namespace>/kernel-cache-<isvc>:<pod-template-hash>`

### 4.1 KernelCacheArtifact

Shared by KCC status and KC spec. A KC `artifact` is immutable after creation.

| Field | Rules |
|---|---|
| `imageReference` | Must be `<repo>@sha256:<64 hex>` |
| `cachePaths[]` | 1 to 64 entries |
| `cachePaths[].containerName` | Defaults to `kserve-container`, then the LLMInferenceService container |
| `cachePaths[].containerPath` | Absolute path. Not `/`, no `.` or `..` segments. Defaults to the `VLLM_CACHE_ROOT` env value, then `/root/.cache/vllm` |
| `cachePaths[].ociPath` | Defaults to `io.vllm.cache`. The API accepts any string that does not start with `/` and has no `..`. The webhook accepts only `io.vllm.cache` and `io.triton.cache` |
| `identity.footprints` | `workloadFootprint`, `compatibilityFootprint` (section 7) |

## 5. Flows

### 5.1 Capture

1. The webhook handles a pod when it has the InferenceService label, has the annotation `serving.kserve.io/kernelcache-supported: "true"` (passed down from the ServingRuntime), and the feature is enabled.
2. The webhook first looks for a cache to reuse (5.3). It injects the sidecar only when none is found. The pod annotation `serving.kserve.io/kernelcache-sidecar-injection` turns injection on or off per pod.
3. The sidecar is injected only for Deployment-backed pods (`pod-template-hash` label), with a resolvable model URI, when the KCC for that revision is not finished yet.
4. The sidecar takes a baseline snapshot, waits for runtime readiness, builds and pushes a delta image, and reports the result (C1, C2, C4).
5. The controller checks the result (digest-pinned, session match, producer pod present), computes the identity from the producer pod, and sets `status.artifact`.
6. With `cert` mode, the controller signs the digest (C6).
7. The controller creates the KC. Node group selection order: pod annotation `serving.kserve.io/kernelcache-nodegroup`, then the single KCNG that matches the producer node, then the default node group. With none of these, no KC is created and a warning event is emitted. `mountType` is taken from `defaultMountType`.

### 5.2 Preparation

1. Every KC reconcile verifies the signature (C6).
2. After verification, the controller creates a prefetch Job for each Ready node in the node group. The Job only mounts the artifact as an image volume, which pulls the image onto the node.
3. The node agent writes per-node state to the KCN, and the controller aggregates it into the KC state.

### 5.3 Reuse

1. Candidates are KCs in the pod namespace that a KCN reports as Ready, whose reported image and footprints match the KC spec, and that passed verification.
2. A KC that matches the workload footprint wins. Otherwise a KC that matches the compatibility footprint is used.
3. The webhook injects the source volume, cache path volumes and the linker for the chosen KC (C8).
4. The pod annotation `internal.serving.kserve.io/kernelcache-usage` records the KC, and KC `status.usage` is built from it.

## 6. Lifecycles

### 6.1 KCC phase

```
Pending -> WaitingForWorkload -> Capturing -> Pushing -> Complete
                                           \----------> Unchanged
(any phase) ------------------------------------------> Failed
```

| Phase | Meaning |
|---|---|
| Complete | Artifact finalized. KC is created after signing |
| Unchanged | No new cache files since the baseline. Nothing pushed |
| Failed | Capture failed |

- `Pushing` is defined in the API, but the sidecar does not report it today.
- If the producer pod goes away during a capture, the KCC is marked ProducerGone. It is kept or deleted per `abandonedCapturePolicy` (`retain` or `delete`) and can be reopened.
- An Unchanged KCC with no artifact is deleted when its producer ends.

### 6.2 KC state

```
Pending -> Preparing -> Ready
   \----------\-------> Error
```

| State | Meaning |
|---|---|
| Pending | No node group, no matching nodes, or no Ready nodes |
| Preparing | Some nodes still preparing |
| Ready | All Ready nodes in the node group are prepared |
| Error | Verification failure, config error, or node preparation error |

### 6.3 KCN entry

```
Pending -> Pulling -> Ready
              \-----> Error
```

- A changed image or footprint on the KC resets the entry to Pending. The entry is removed when the KC is deleted.
- `Extracting` is defined in the API but not used today.

## 7. Matching

| Footprint | Inputs | Meaning |
|---|---|---|
| compatibility | Runtime image, model URI hash, runtime options that affect the cache (tensor/pipeline parallel size, max-model-len, dtype, quantization, kv-cache-dtype, block-size, ...) | The same cache is valid under these conditions |
| workload | All of the above plus namespace, workload kind and name, command/args hashes | The cache comes from the same workload |

| Runtime image reference | Footprints produced |
|---|---|
| digest | both |
| tag (not `latest`) | compatibility only |
| `latest` or no tag | none (not a reuse candidate) |

The model URI hash drops user info, query and fragment first. A workload match compares both footprints.

## 8. Contracts

The components run as different processes, in different images and languages: Go controllers, a Python entrypoint, the Go MCV CLI and a shell linker. These are the contracts at each boundary.

### C1. Webhook -> MCV sidecar: capture config

Passed as env on the sidecar container.

| Env | Content |
|---|---|
| `MCV_CAPTURE_MODE` | `true` |
| `MCV_CAPTURE_CONFIG` | `{version: 1, cacheDir, targetImage, capture{name, namespace, sessionID}, cachePaths[]}` |
| `MCV_READINESS_CONFIG` | `{url, mcvCaptureReadinessTimeoutSeconds}` |
| `MCV_RUNTIME_INFO` | `{commandHash, argsHash, modelURIHash}` |
| `MCV_SOURCE_POD_NAME` | Downward API |
| `MCV_REPORTER_ACCESS_FILE`, `MCV_KUBERNETES_CA_FILE` | Reporter token and CA paths |
| `MCV_REGISTRY_ACCESS_FILE`, `MCV_REGISTRY_ALLOWED_ENDPOINT` | Push credentials (serviceAccountToken auth) |
| `MCV_REGISTRY_INSECURE`, `MCV_REGISTRY_CA_FILE` | Registry TLS settings |

Mounts:

- For cache path i, emptyDir `mcv-cache-i` is mounted at `containerPath` in the runtime container and at `/workspace/cache/i` in the sidecar.
- Reporter credentials at `/var/run/secrets/mcv-reporter`, registry credentials at `/var/run/secrets/mcv-registry`.

Rules:

- `cacheDir` is `/workspace/cache/0`. Capture works on that one directory.
- sessionID is a new UUID per pod.
- The readiness URL is the runtime container HTTP readinessProbe, rewritten to loopback. No HTTP readinessProbe means the pod is denied.
- No registry endpoint in config means the pod is denied.
- The sidecar is a regular container in `spec.containers` and starts together with the runtime.

### C2. Sidecar -> MCV CLI

| Step | Call |
|---|---|
| baseline | `mcv --snapshot --dir <cacheDir> --snapshot-file <file>` (3 retries) |
| build and push | `mcv --create --builder oci --image <target> --dir <cacheDir> --result <file> --delta-from-snapshot --snapshot-file <file>` |

Result file:

```json
{"state": "Succeeded|Unchanged", "imageReference": "<repo>@sha256:...", "cacheSizeBytes": 0, "completedAt": "..."}
```

Rules:

- No change gives `Unchanged` and no push.
- When only new directories appeared, a directory-only delta image is built. Otherwise a full image is built.

### C3. MCV -> artifact: OCI image

Layout (single layer):

```
io.<type>.cache/...               cache files
io.<type>.manifest/manifest.json  MCV metadata
```

- type is `vllm`, `triton` or `habana`. Detection checks vLLM, then Triton, then Habana, and picks one.
- Config labels: `cache.<type>.image/*` (entry-count, cache-size-bytes, summary, format) and `io.kserve.km/*` (framework, cache-type, cache-hash, cache-mount-subpath, cache-root-env).
- Media types: OCI manifest and OCI config.

Consumers of the layout:

- The KServe consumer uses only the top-level directory name (`<ociPath>`).
- The KServe control plane does not read labels or the manifest.
- `mcv --extract` reads them: the summary label to detect the type, a GPU compatibility check unless `--no-gpu`, and the size label to check the result.

`mcv --extract` rules:

- Extracts the contents of `io.<type>.cache/` into `--dir` as is.
- Rejects path traversal.
- Creates only regular files and directories. No symlinks or hardlinks.
- 10 GiB limit per file.

### C4. Sidecar -> controller: result report

The sidecar does GET and PUT on the KCC `/status` subresource with a bearer token, and writes `status.runtimeResult` (string map).

| Key | Content |
|---|---|
| `state` | `WaitingForWorkload`, `Capturing`, `Succeeded`, `Unchanged`, `Failed` |
| MCV result | `imageReference`, `cacheSizeBytes`, `completedAt` |
| Other | `cachePaths`, `runtimeInfo`, `sourcePodName`, `captureSessionID`, `capturedAt`, `reason` on failure |

Access:

- Each KCC has its own ServiceAccount, Role, RoleBinding and Secret. The token comes from TokenRequest with a 600s lifetime.
- The validating webhook lets a reporter SA write only its own KCC, change only `runtimeResult`, not after a terminal phase, and the first `sourcePodName` claim wins.

Decision:

- The report is input only. The controller decides: digest-pinned `imageReference`, matching activeSession, producer pod present.
- `state` to phase: `Succeeded` -> Complete, `Unchanged` -> Unchanged, `Failed` or unknown -> Failed.

### C5. KCC -> KC: artifact

- A KC is created when the KCC has `Ready=True` at the current generation, the session matches, the phase is Complete with an artifact, and signing is Succeeded (or Skipped when the mode is none).
- KC `spec.artifact` is a copy of KCC `status.artifact` and is immutable.
- The KC has no ownerReference to the KCC.

### C6. Signing and verification

| Item | Rule |
|---|---|
| Modes | `none`, `cert` (default `cert`) |
| Failure policy | `reject` |
| Signer | Controller. Signing key in a Secret in the KServe namespace (issued by cert-manager) |
| Signed object | Artifact digest. Cosign format, no transparency log |
| When verified | Every KC reconcile |
| Verification | CA bundle chain and subject pattern. The verified digest must equal the digest in `imageReference` |
| Use | The consumer uses a KC only when `status.verification` is Succeeded and verified for the current mode |

### C7. KC -> node preparation

- KC `nodeGroupRef` points to a KCNG.
- Prefetch Jobs run in `jobNamespace`, one per Ready node in the KCNG, placed with a `kubernetes.io/hostname` nodeSelector and the KCNG tolerations. The Job runs `prefetchImage` with the artifact mounted as an image volume and only runs `test -d`.
- The node agent writes KCN `cacheStatus["<ns>/<name>"]` (`imageReference`, `footprints`, `state`, `message`) from Job status and `node.status.images`.
- The consumer trusts a Ready entry only when its `imageReference` and `footprints` equal the KC spec.
- No placement constraint (affinity, nodeSelector) is added to consumer pods.

### C8. Consumer injection -> pod (delivery)

With `mountType: oci`:

| Item | Content |
|---|---|
| Source volume | `kernel-cache-source`: image volume (`imageReference`, PullIfNotPresent) |
| Runtime mounts | Source mounted read-only at `/var/run/gkm/kernel-cache`. For cache path i, emptyDir `kernel-cache-runtime-<i>` at `containerPath` (an existing writable mount at that exact path is reused) |
| Linker | Init container `kernel-cache-linker`, prepended. Image is `prefetchImage` |
| Linker args | List of `(source, target)` pairs: `source=/var/run/gkm/kernel-cache/<ociPath>`, `target=containerPath` |
| Usage record | Annotation `internal.serving.kserve.io/kernelcache-usage=<ns>/<kc>` |

Linker:

```sh
set -eu
# for each (source, target):
#   missing source directory -> exit 1
mkdir -p "$target"
cp -sr "$source"/. "$target"/
```

Directories under `target` are real directories. Files are symlinks into the read-only source.

### C9. Configuration

The `kernelcache` key of the `inferenceservice-config` ConfigMap. Invalid values are rejected when the config is loaded.

| Field | Default | Rules |
|---|---|---|
| `enabled` | false | |
| `defaultSidecarInjection` | true | |
| `defaultMountType` | `oci` | Only `oci` |
| `defaultNodeGroup` | "" | |
| `jobNamespace` | `kserve-kernelcache-jobs` | Must exist |
| `mcvImage` | `kserve/kserve-mcv:latest-minimal` | |
| `prefetchImage` | `registry.access.redhat.com/ubi9/ubi-minimal:latest` | Used by prefetch Jobs and the linker |
| `runtimeImagePrefetch` | disabled | Also pre-pulls the runtime image |
| `mcvCaptureReadinessTimeoutSeconds` | 600 | Must be > 0 |
| `abandonedCapturePolicy` | `retain` | `retain` or `delete` |
| `reconcileIntervalSeconds`, `jobTTLSecondsAfterFinished` | 300, 600 | |
| `registry` | | `endpoint`, `insecure`, `caConfigMapRef`, `auth{type: none or serviceAccountToken, tokenTTLSeconds, pushRoleRef, pullRoleRef}`. `insecure` cannot be combined with a CA or serviceAccountToken |
| `artifactSecurity` | `cert` | `mode`, `failurePolicy`, `cert{signingProfileRef, trustBundle, trustBundleKey, subjectRegexp}` |

Default cache paths are not configurable. They follow 4.1.

## 9. Security model

| Boundary | Threat | Control |
|---|---|---|
| Artifact in the registry | Swap or tamper | Digest references, cert signing, verification on every KC reconcile |
| Sidecar report | Touching another capture, forged results | Per-capture SA and short-lived token, validating webhook, controller makes the final decision |
| Push credentials | Broad write access | Issued per capture (serviceAccountToken auth), allowed endpoint check |
| Signing key | Exposure | Only in the control plane namespace. Sidecars and nodes cannot sign |
| Consumer cache | Runtime changes the cache and affects other pods | Source is a read-only image. No shared writable path between pods |
| Namespace isolation | Using another namespace's cache | Only KCs in the pod namespace are candidates |

## 10. Failure handling

| Case | Behavior |
|---|---|
| No cache to reuse, unknown model source, floating runtime image | Continue without a cache (sidecar injected if eligible) |
| Error while selecting or injecting a consumer cache | Pod denied (webhook `failurePolicy: Fail`) |
| Error while injecting the sidecar (no HTTP readinessProbe, no registry endpoint, ...) | Pod denied |
| Config load error | Pod denied |
| Invalid `kernelcache-sidecar-injection` value | Pod denied (400) |
| Capture failure | KCC Failed. The pod keeps running |
| Verification failure | KC Error. No prefetch, not a reuse candidate |
| Linker source missing | Init container fails, pod does not start |

## 11. Code map

| Area | Location |
|---|---|
| API | `pkg/apis/serving/v1alpha1/kernel_cache_*.go` |
| Config | `pkg/apis/serving/v1beta1/configmap.go` (`KernelCacheConfig`) |
| Webhooks | `pkg/webhook/admission/kernelcache/` |
| Controllers | `pkg/controller/v1alpha1/kernelcache/reconcilers/` |
| Node agent | `cmd/kernelcachenode/`, `pkg/controller/v1alpha1/kernelcachenode/` |
| Shared | `pkg/kernelcache/` (container, captureconfig, identity, podconfig, reporter, security) |
| MCV | `kernelcache/mcv/` (`capture-entrypoint.py`, `cmd/`, `pkg/`, `docs/`) |
| e2e | `test/e2e/kernelcache/` |
