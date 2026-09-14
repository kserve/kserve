# Model Cache Vault (MCV)

<img src="logo/mcv.png" alt="mcv" width="20%" height="auto">

A Model/GPU kernel cache container packaging utility inspired by
[WASM](https://github.com/solo-io/wasm/blob/master/spec/README.md).

**Original Authors:**
- [Maryam Tahhan](https://github.com/maryamtahhan)
- [Alessandro Sangiorgi](https://github.com/fulvius31)

## Features

- Build container images containing GPU Kernel/Model caches.
- Extract a cache from an OCI image
- Compatible with docker or buildah
- **Single-layer image output** (squashed) for cosign compatibility
- Client API for retrieving and extracting images
- Artifact and image signing via cosign (indirectly)

### Kernel Cache artifact and image signing

- Cache artifact signing with Cosign
- Container image signing support with Cosign
- **Single-layer images**: MCV uses a multi-stage `FROM scratch` build to squash cache content into a single rootfs layer, compatible with cosign signing and verification. Works with Docker (BuildKit) and Buildah without experimental features

## Build Instructions

### Requirements

- Go 1.25.0 or later
- GCC, if on OSx, make sure to install GNU GCC: `brew install gcc`. Plus it requires `CGO_ENABLED=1`

### Install dependencies

```bash
sudo dnf install gpgme-devel
sudo dnf install btrfs-progs-devel
```

Build the binary:

```bash
make build
```

After the binary is built, it can be found in an arch specific directory,
something like `./_output/bin/linux_amd64/mcv`. To install the binary in
the local `~/go/bin` directory, run (make sure `~/go/bin` is in $PATH):

```bash
make install
```

## Usage

Below is the `mcv` usage:

```bash
$ mcv -h
A Model cache container image management utility

Usage:
  mcv [flags]

Flags:
  -c, --create               Create OCI image from cache directory
  -e, --extract              Extract Triton/vLLM cache from OCI image
  -i, --image string         OCI image name (required for create, extract, check-compat)
  -d, --dir string           Triton/vLLM cache directory path
      --gpu-info             Display GPU-specific information
      --check-compat         Check GPU compatibility with specified image
  -b, --baremetal            Enable detailed baremetal preflight checks
  -l, --log-level string     Set logging verbosity (debug, info, warning, error) (default "info")
      --builder string       Specify the builder to use (buildah or docker)
      --no-gpu               Disable GPU detection and preflight checks (for testing)
      --stub                 Use mock/stub data for hardware info (for testing)
  -t, --timeout int          Timeout in minutes for hardware detection operations (0 = disable) (default 10)
      --version              Display the version of the application
  -h, --help                 help for mcv
```

## Dependencies

- [buildah dependencies](https://github.com/containers/buildah/blob/main/install.md#building-from-scratch)

## GPU Kernel Image Container Specification

### Cache Image Container Specification

The Cache Image specification defines how to bundle caches as container
images. A compatible Cache image consists of cache directory for a Triton
Kernel/vLLM model. The details can be found in
[spec-compat.md](./docs/spec-compat.md)

### vLLM Binary Cache Support

MCV supports both legacy (triton cache) and new (binary cache) vLLM formats:

1. **vLLM Triton Cache Format** (legacy) - Stores `triton_cache/` and
   `inductor_cache/` inside rank directories
2. **vLLM Binary Cache Format** (new) - Stores prefix directories
   (e.g., `backbone/`) inside rank directories

For detailed information about vLLM binary cache support, see:
[vllm-binary-cache.md](./docs/vllm-binary-cache.md)

### Triton Cache Example

To extract the Triton Cache for the
[01-vector-add.py](https://github.com/triton-lang/triton/blob/main/python/tutorials/01-vector-add.py)
tutorial from [Triton](https://github.com/triton-lang/triton), run the
following:

```bash
mcv -e -i quay.io/gkm/vector-add-cache:rocm
Img fetched successfully!!!!!!!!
Img Digest: sha256:b6d7703261642df0bf95175a64a01548eb4baf265c5755c30ede0fea03cd5d97
Img Size: 525
bash-4.4#
```

This will extract the cache directory from the
`quay.io/gkm/vector-add-cache:rocm` container image and copy it to
`~/.triton/cache/`.

To Create an OCI image for a Triton Cache using docker run the
following:

```bash
mcv -c -i quay.io/gkm/vector-add-cache:rocm -d example/vector-add-cache-rocm
INFO[2026-07-27 21:37:05] Setting log level: info
INFO[2026-07-27 21:37:05] Using docker to build the image
INFO[2026-07-27 21:37:05] Detected cache components: [triton]
INFO[2026-07-27 21:37:05] Dockerfile generated successfully at /tmp/.mcv/docker/Dockerfile
{"stream":"Step 1/9 : FROM scratch AS build"}
{"stream":"\n"}
{"stream":" ---\u003e \n"}
{"stream":"Step 2/9 : COPY \"./io.triton.cache/\" \"./io.triton.cache/\""}
{"stream":"\n"}
{"stream":" ---\u003e aa1fa6bcd3db\n"}
{"stream":"Step 3/9 : COPY \"./io.triton.manifest/manifest.json\" \"./io.triton.manifest/manifest.json\""}
{"stream":"\n"}
{"stream":" ---\u003e 57d1c4815d2e\n"}
{"aux":{"ID":"sha256:57d1c4815d2ede3d2ed3b64a9e5f062577ca8dd501c8b59c5d90b3351bd06b47"}}
{"stream":"Step 4/9 : FROM scratch"}
{"stream":"\n"}
{"stream":" ---\u003e \n"}
{"stream":"Step 5/9 : LABEL org.opencontainers.image.title=vector-add-cache"}
{"stream":"\n"}
{"stream":" ---\u003e Running in 6e7dce4e97bc\n"}
{"stream":" ---\u003e e8b4014c2ae2\n"}
{"stream":"Step 6/9 : COPY --from=build / /"}
{"stream":"\n"}
{"stream":" ---\u003e ddcb3cce60a7\n"}
{"stream":"Step 7/9 : LABEL cache.triton.image/cache-size-bytes=80415"}
{"stream":"\n"}
{"stream":" ---\u003e Running in 1e196c7ace14\n"}
{"stream":" ---\u003e 56aa910decff\n"}
{"stream":"Step 8/9 : LABEL cache.triton.image/entry-count=1"}
{"stream":"\n"}
{"stream":" ---\u003e Running in af4bb8ba633c\n"}
{"stream":" ---\u003e ce2af41bccb7\n"}
{"stream":"Step 9/9 : LABEL cache.triton.image/summary={\"targets\":[{\"backend\":\"hip\",\"arch\":\"gfx90a\",\"warp_size\":64}]}"}
{"stream":"\n"}
{"stream":" ---\u003e Running in d97c0447e121\n"}
{"stream":" ---\u003e 170e5776a1f5\n"}
{"aux":{"ID":"sha256:170e5776a1f56a8e8e3a8a4398aaf814e196f72001d9a63aabcc3055ecd238ae"}}
{"stream":"Successfully built 170e5776a1f5\n"}
{"stream":"Successfully tagged quay.io/gkm/vector-add-cache:rocm\n"}
INFO[2026-07-27 21:37:06] Docker image built successfully
INFO[2026-07-27 21:37:06] OCI image created successfully.
```

To see the new image:

```bash
docker images

IMAGE                               ID             DISK USAGE   CONTENT SIZE   EXTRA
quay.io/gkm/vector-add-cache:rocm   170e5776a1f5        136kB         21.2kB
```

To verify the image has a single layer (important for cosign compatibility):

```bash
docker inspect quay.io/gkm/vector-add-cache:rocm | jq '.[0].RootFS.Layers | length'
1
```

To inspect the docker image with Skopeo (note the **single layer** due to squashing):

> **Note**: Use `docker-daemon:` prefix for Docker images, not `containers-storage:` (which is for Buildah/Podman).

```bash
skopeo inspect docker-daemon:quay.io/gkm/vector-add-cache:rocm
{
    "Name": "quay.io/gkm/vector-add-cache",
    "Digest": "sha256:97bd4cb83b692bebed5adc4cd92647478052719e4c7771562af31d1aef198cb8",
    "RepoTags": [],
    "Created": "2026-07-27T21:37:05.971275986-04:00",
    "DockerVersion": "",
    "Labels": {
        "cache.triton.image/cache-size-bytes": "80415",
        "cache.triton.image/entry-count": "1",
        "cache.triton.image/summary": "{\"targets\":[{\"backend\":\"hip\",\"arch\":\"gfx90a\",\"warp_size\":64}]}",
        "org.opencontainers.image.title": "vector-add-cache"
    },
    "Architecture": "amd64",
    "Os": "linux",
    "Layers": [
        "sha256:4d49b8253e60536d82418c622032c65ab3f31235e92b6d12cb29a9131c2aef04"
    ],
    "LayersData": [
        {
            "MIMEType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
            "Digest": "sha256:4d49b8253e60536d82418c622032c65ab3f31235e92b6d12cb29a9131c2aef04",
            "Size": 93184,
            "Annotations": null
        }
    ],
    "Env": [
        "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
    ]
}
```

> **Note**: If `buildah` is installed it will be favoured to build the image.
The build output is shown below.

```bash
mcv -c -i quay.io/gkm/vector-add-cache:rocm -d tests/vector-add-cache-rocm
INFO[2025-05-28 12:23:04] baremetalFlag false
INFO[2025-05-28 12:23:04] Using buildah to build the image
INFO[2025-05-28 12:23:04] Wrote manifest to /tmp/buildah-manifest-dir-2780945232/manifest.json
INFO[2025-05-28 12:23:04] Image built! baadff55392c0ada6f0d358c255d63ca770fb20b87429a732480e00bbf8d044b
INFO[2025-05-28 12:23:04] Temporary directories successfully deleted.
INFO[2025-05-28 12:23:04] OCI image created successfully.
```

To inspect the buildah image with Skopeo

```bash
skopeo inspect containers-storage:quay.io/gkm/vector-add-cache:rocm
{
    "Name": "quay.io/gkm/vector-add-cache",
    "Digest": "sha256:3f8c7b3aeeffd9ee3f673486f3bc681a7f9ed39e21242628e6845755191d6bd4",
    "RepoTags": [],
    "Created": "2025-05-28T15:45:17.379786001Z",
    "DockerVersion": "",
    "Labels": {
        "cache.triton.image/cache-size-bytes": "80415",
        "cache.triton.image/entry-count": "1",
        "cache.triton.image/summary": "{\"targets\":[{\"backend\":\"hip\",\"arch\":\"gfx90a\",\"warp_size\":64}]}"
    },
    "Architecture": "amd64",
    "Os": "linux",
    "Layers": [
        "sha256:ef89050f71ecc3dc925f14c12d2fd406c067f78987eed36a1176b19499c8ea20"
    ],
    "LayersData": [
        {
            "MIMEType": "application/vnd.oci.image.layer.v1.tar",
            "Digest": "sha256:ef89050f71ecc3dc925f14c12d2fd406c067f78987eed36a1176b19499c8ea20",
            "Size": 93184,
            "Annotations": null
        }
    ],
    "Env": null
}
```

To inspect the image labels specifically run:

```bash
skopeo inspect containers-storage:quay.io/gkm/vector-add-cache:rocm | jq -r '.Labels["cache.triton.image/summary"]' | jq .
{
  "targets": [
    {
      "backend": "hip",
      "arch": "gfx90a",
      "warp_size": 64
    }
  ]
}
```

### vLLM Cache example

To Create an OCI image for a vLLM Cache run the following:

```bash
mcv -c -i quay.io/gkm/cache-examples:vllm-example -d tests/vllm-cache
INFO[2025-09-03 09:04:15] Hardware accelerator(s) detected (2). GPU support enabled.
INFO[2025-09-03 09:04:15] Using buildah to build the image
INFO[2025-09-03 09:04:23] Detected cache components: [vllm]
INFO[2025-09-03 09:04:24] Image built! 8218fac0225882a7de7a1f11f32aff25df2936f1f12b08c0c26ab30897d19c5a
INFO[2025-09-03 09:04:24] OCI image created successfully.
```

To inspect the image labels specifically run:

```bash
skopeo inspect containers-storage:quay.io/gkm/cache-examples:vllm-example
{
    "Name": "quay.io/gkm/cache-examples",
    "Digest": "sha256:9e731d58adccd608cb18dcefe259acd30ffe976d5e98208a4158ce22c0b5d1e2",
    "RepoTags": [],
    "Created": "2026-02-10T12:04:38.260317569Z",
    "DockerVersion": "",
    "Labels": {
        "cache.vllm.image/cache-size-bytes": "2269180",
        "cache.vllm.image/entry-count": "1",
        "cache.vllm.image/summary": "{\"targets\":[{\"backend\":\"hip\",\"arch\":\"gfx90a\",\"warp_size\":64}]}"
    },
    "Architecture": "amd64",
    "Os": "linux",
    "Layers": [
        "sha256:440b5cbd3b76dc17a6012e17fc56341d4894b88ab7a85b12c5e2f6f7c4b80661"
    ],
    "LayersData": [
        {
            "MIMEType": "application/vnd.oci.image.layer.v1.tar+gzip",
            "Digest": "sha256:440b5cbd3b76dc17a6012e17fc56341d4894b88ab7a85b12c5e2f6f7c4b80661",
            "Size": 250291,
            "Annotations": null
        }
    ],
    "Env": null
}
```

To extract the vLLM Cache run the following:

```bash
mcv -e -i quay.io/gkm/cache-examples:vllm-example
INFO[2025-09-03 09:06:00] Hardware accelerator(s) detected (2). GPU support enabled.
INFO[2025-09-03 09:06:02] Preflight GPU compatibility check passed.
INFO[2025-09-03 09:06:02] Preflight completed                           matched="[0 1]" unmatched="[]"
INFO[2025-09-03 09:06:04] Extracting cache to directory: /home/fedora/.cache/vllm
```

## Signing Container Images

Use [Sigstore Cosign](https://docs.sigstore.dev/) to sign mcv-built images.

1. Install Cosign

```bash
go install github.com/sigstore/cosign/v2/cmd/cosign@latest
```

2. Sign an image

```bash
cosign sign -y quay.io/tkm/vector-add-cache@sha256:<digest>
⏎
Generating ephemeral keys...
Retrieving signed certificate...

    The sigstore service, hosted by sigstore a Series of LF Projects,
    LLC, is provided pursuant to the Hosted Project Tools Terms of
    Use, available at
    https://lfprojects.org/policies/hosted-project-tools-terms-of-use/.
    Note that if your submission includes personal data associated with
    this signed artifact, it will be part of an immutable record.
    This may include the email address associated with the account with
    which you authenticate your contractual Agreement.
    This information will be used for signing this artifact and will be
    stored in public transparency logs and cannot be removed later, and
    is subject to the Immutable Record notice at
    https://lfprojects.org/policies/hosted-project-tools-immutable-records/.

By typing 'y', you attest that (1) you are not submitting the personal
data of any other person; and (2) you understand and agree to the
statement and the Agreement terms at the URLs listed above.
Your browser will now be opened to:
...
```

Cosign will prompt you to authenticate and display legal terms regarding
transparency logs.

3. Confirm and Finish
    - Ephemeral keys will be generated
    - Signature will be pushed to the registry
    - You'll see a success message including the transparency log index

Upon successful completion, you will see an output similar to:

```bash
Successfully verified SCT...
tlog entry created with index: 215011903
Pushing signature to: quay.io/gkm/cache-examples
```

## MCV Client API

### Extracting a Cache from a Container Image

An example snippet of how to use the client API to extract a Cache from a
container image is shown below.

```go
package main

import (
    "fmt"

    "github.com/kserve/kserve/kernelcache/mcv/pkg/client"
)

func main() {
    matched, unmatched, err := client.ExtractCache(client.Options{
        ImageName:       "quay.io/gkm/cache-examples:vector-add-cache-cuda",
        CacheDir:        "/tmp/testcache",
        LogLevel:        "debug",
        EnableBaremetal: nil, // or false if explicitly desired
    })
    if err != nil {
        panic(err)
    }
    fmt.Printf("Matched GPU IDs: %v, Unmatched GPU IDs: %v\n", matched, unmatched)
}
```

### Detecting System GPU Devices

You can also use the MCV client API to retrieve details about the system's
available GPUs:

```go
package main

import (
    "encoding/json"
    "fmt"
    "log"

    "github.com/kserve/kserve/kernelcache/mcv/pkg/client"
)

func main() {
    stub := false
    gpus, err := client.GetSystemGPUInfo(client.HwOptions{EnableStub: &stub})
    if err != nil && gpus == nil {
        log.Fatalf("Error retrieving GPU info: %v", err)
    }

    output, err := json.MarshalIndent(gpus, "", "  ")
    if err != nil {
        log.Fatalf("Failed to format GPU info: %v", err)
    }

    fmt.Println("Detected GPU Devices:")
    fmt.Println(string(output))
}
```

### Checking Image Compatibility with Host GPUs

```go
package main

import (
    "fmt"
    "log"

    "github.com/kserve/kserve/kernelcache/mcv/pkg/client"
)

func main() {
    matched, unmatched, err := client.PreflightCheck(
        "quay.io/gkm/cache-examples:vector-add-cache-cuda")
    if err != nil {
        log.Fatalf("Preflight check failed: %v", err)
    }

    fmt.Printf("Compatible GPUs: %d\n", len(matched))
    for i, gpu := range matched {
        fmt.Printf("  MATCH %d: Backend=%s, Arch=%s, WarpSize=%d, "+
            "PTX=%s\n", i, gpu.Backend, gpu.Arch, gpu.WarpSize,
            gpu.PTXVersion)
    }

    fmt.Printf("Incompatible GPUs: %d\n", len(unmatched))
    for i, gpu := range unmatched {
        fmt.Printf("  NO-MATCH %d: Backend=%s, Arch=%s, WarpSize=%d, "+
            "PTX=%s\n", i, gpu.Backend, gpu.Arch, gpu.WarpSize,
            gpu.PTXVersion)
    }
}
```

### Static Device Configuration (Stub Mode)

MCV supports running in environments without GPUs by using a static device
configuration. This is useful for testing or CI environments.

#### Stub Mode Usage

Run MCV with the `--stub` flag. It will use the static config and behave as
if those devices are present.

## Container Image Variants

MCV is built as a multi-target Dockerfile with five variants, each tailored
to a specific GPU environment:

| Variant | Target | Base Image | GPU Support | Make Target |
|---------|--------|------------|-------------|-------------|
| **minimal** | `mcv-minimal` | `debian:bookworm-slim` | None (use `--no-gpu`) | `make docker-build-mcv-minimal` |
| **rocm** | `mcv-rocm` | `debian:bookworm-slim` | AMD (ROCm, `amd-smi`, `rocm-smi`) | `make docker-build-mcv-rocm` |
| **gaudi** | `mcv-gaudi` | `ubuntu:24.04` | Intel Gaudi (`hl-smi`) | `make docker-build-mcv-gaudi` |
| **cuda** | `mcv-cuda` | `nvidia/cuda:12.6.3-base-ubuntu24.04` | NVIDIA (CUDA, NVML) | `make docker-build-mcv-cuda` |
| **unified** | `mcv-unified` | `nvidia/cuda:12.6.3-base-ubuntu24.04` | All vendors (NVIDIA + AMD + Gaudi) | `make docker-build-mcv-unified` |

Build all variants at once:

```bash
make docker-build-mcv
```

All images run as non-root (`appuser`, UID 1000) and use the `vfs` storage
driver for rootless buildah operation.

## Using MCV image to build cache images

MCV provides container images that can be used to wrap a vLLM/Triton cache
in an OCI container image that can then be pushed to a container registry
(without having to install mcv locally). Use the **minimal** variant for
cache creation (`--create`, `--no-gpu`) and a GPU-specific variant for
extraction with preflight checks. These images can also be used as part of a
[github workflow](../../.github/workflows/mcv-build-test.yml).

### MCV container image with docker

To use docker on the host with an MCV image, you need to mount the cache
directory to the container and run the following command:

```bash
docker run --rm -it --privileged \
  -v <path-to-cache>/example:/example \
  kserve/kserve-mcv:latest-minimal bash -lc '
    /mcv -c -i quay.io/gkm/vector-add-cache:rocm \
        -d /example/vector-add-cache-rocm --no-gpu &&
    buildah push containers-storage:quay.io/gkm/vector-add-cache:rocm \
        docker-archive:/example/vector-add-cache-rocm.tar:quay.io/gkm/vector-add-cache:rocm
  '
INFO[2025-09-11 16:46:54] Setting log level: info
INFO[2025-09-11 16:46:54] Using buildah to build the image
INFO[2025-09-11 16:46:54] Detected cache components: [triton]
INFO[2025-09-11 16:46:55] Image built! 8ce4bc2e98abfa8c0a5a6f6046c1c7bc8ac09805ecb029427a995dc2897828f8
INFO[2025-09-11 16:46:55] OCI image created successfully.
Getting image source signatures
Copying blob 24b82d6fef87 done
Copying config 8ce4bc2e98 done
Writing manifest to image destination
Storing signatures
```

Then on host:

```bash
docker load -i <path-to-cache>/example/vector-add-cache-rocm.tar
24b82d6fef87: Loading layer  93.18kB/93.18kB
The image quay.io/gkm/vector-add-cache:rocm already exists, renaming
the old one with ID sha256:5dc90b88f536e44e186c5a076afbb7a54389aed6f0ddfa21365ae2c7f79cb21d to empty string
Loaded image: quay.io/gkm/vector-add-cache:rocm
```

Check the images:

```bash
docker images
REPOSITORY                               TAG       IMAGE ID       CREATED          SIZE
quay.io/gkm/vector-add-cache             rocm      8ce4bc2e98ab   15 seconds ago   80.7kB
```

### MCV container image with podman

To use podman on the host with an MCV image, you need to mount the cache
directory to the container and run the following command:

```bash
podman run --rm -it --privileged \
  -v <path-to-cache>/example:/example \
  quay.io/gkm/mcv bash -lc '
    /mcv -c -i quay.io/gkm/vector-add-cache:rocm \
        -d /example/vector-add-cache-rocm &&
    buildah push containers-storage:quay.io/gkm/vector-add-cache:rocm \
        oci-archive:/example/vector-add-cache-rocm.oci:quay.io/gkm/vector-add-cache:rocm
  '
```

```bash
podman load -i <path-to-cache>/example/vector-add-cache-rocm.oci
```

```bash
podman images
REPOSITORY                                TAG                         IMAGE ID      CREATED         SIZE
quay.io/gkm/vector-add-cache              rocm                        b1bc2ae6bef1  25 seconds ago  94.7 kB
```
