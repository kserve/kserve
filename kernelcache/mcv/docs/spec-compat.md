
# GPU Kernel Cache Image Specification v0.0.0

## Introduction

This document describes a variant of Triton/vLLM Artifact Image Specification
which leverages the compatible layer media types. We call this variant "compat".

## Description

This *compat* variant makes use of compatible media type for layers, and is not
based on custom OCI Artifact media types. This way users can operate with
standard tools such as docker, podman, buildah, and standard container
registries which don't yet support custom media types.

## Specification

### Layer

The *compat* variant must have the 1 layer whose media type is one of the
following:

- `application/vnd.oci.image.layer.v1.tar+gzip`
- `application/vnd.docker.image.rootfs.diff.tar.gzip`

In addition, such a layer must consist of the Triton/vLLM cache directory
contents.

### Annotation

If the media type equals `application/vnd.oci.image.layer.v1.tar+gzip`, then a
*compat* variant image *should* add the annotation `cache.triton.image/variant=compat`
or `cache.vllm.image/variant=compat` in the manifest to make it easy to distinguish
this *compat* variant from the *oci* variant. Note that this is **optional**.

### Example with `application/vnd.oci.image.layer.v1.tar+gzip` media type

The following is an example OCI manifest of images with
`application/vnd.oci.image.layer.v1.tar+gzip` layer media type:

```bash
$ skopeo inspect docker://quay.io/tkm/triton-cache:01-vector-add-latest
{
    "Name": "quay.io/tkm/triton-cache",
    "Digest": "sha256:6b869186b227d5819441d796a55ebed19b961a6143e5c7bbcd05d69b78f4cd29",
    "RepoTags": [
        "01-vector-add-latest"
    ],
    "Created": "2024-12-17T12:05:39.704993297Z",
    "DockerVersion": "",
    "Labels": {
        "io.buildah.version": "1.33.11",
        "org.opencontainers.image.title": "01-vector-add-latest"
    },
    "Architecture": "amd64",
    "Os": "linux",
    "Layers": [
        "sha256:529cec732c6bfcd7ec14c620ecd89cd338578a34129c57d33ec3f30f9c4a069c"
    ],
    "LayersData": [
        {
            "MIMEType": "application/vnd.oci.image.layer.v1.tar+gzip",
            "Digest": "sha256:529cec732c6bfcd7ec14c620ecd89cd338578a34129c57d33ec3f30f9c4a069c",
            "Size": 18871,
            "Annotations": null
        }
    ],
    "Env": [
        "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
    ]
}
```

> **Note**: The same can be done for a vLLM cache.

### Example with `application/vnd.docker.image.rootfs.diff.tar.gzip` media type

The following is an example Docker manifest of images with
`application/vnd.docker.image.rootfs.diff.tar.gzip` layer media type:

```bash
$ skopeo inspect docker://quay.io/tkm/triton-cache:01-vector-add-latest
{
    "Name": "quay.io/tkm/triton-cache",
    "Digest": "sha256:b6d7703261642df0bf95175a64a01548eb4baf265c5755c30ede0fea03cd5d97",
    "RepoTags": [
        "01-vector-add-latest"
    ],
    "Created": "2024-12-17T15:30:17.139084969Z",
    "DockerVersion": "",
    "Labels": {
        "org.opencontainers.image.title": "01-vector-add-latest"
    },
    "Architecture": "amd64",
    "Os": "linux",
    "Layers": [
        "sha256:1ad665f418818c34ae56491dff3949c31f81a0e089c2e7b95053aaf4e299f452"
    ],
    "LayersData": [
        {
            "MIMEType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
            "Digest": "sha256:1ad665f418818c34ae56491dff3949c31f81a0e089c2e7b95053aaf4e299f452",
            "Size": 18107,
            "Annotations": null
        }
    ],
    "Env": [
        "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
    ]
}
```

## Appendix 1: build a Triton *compat* image with Buildah

We demonstrate how to build a *compat* image with Buildah, a standard cli
for building OCI images. We use v1.21.0 of Buildah here. Produced images
have `application/vnd.oci.image.layer.v1.tar+gzip` layer media type

We assume that you have a Triton cache that you want to package as an image.

1. First, we create a working container from `scratch` base image with
`buildah ... from` command.

```bash
buildah --name quay.io/tkm/triton-cache:01-vector-add-latest from scratch
```

1. Next, add the annotation described above via `buildah config` command

```bash
buildah config --annotation "module.triton.image/variant=compat" quay.io/tkm/triton-cache:01-vector-add-latest
```

> Note: This step is optional. See [Annotation](#annotation) section.

1. Then copy the files into that base image by `buildah copy` command
to create the layer.

```bash
buildah copy quay.io/tkm/triton-cache:01-vector-add-latest vector-add-cache/ ./io.triton.cache
612fd1391d341bcb9f738a4d0ed6a15095e68dfc3245d8a899af3ecb4b60b8b1
```

> **Note**: you must execute `buildah copy` exactly once in order to end
> up having only one layer in produced images**

1. Now, you can build a *compat* image and push it to your registry
via `buildah commit` command

```bash
buildah commit quay.io/tkm/triton-cache:01-vector-add-latest docker://quay.io/tkm/triton-cache:01-vector-add-latest
```

> **Note**: The same can be done for a vLLM cache.

## Appendix 2: build a Triton *compat* image with Docker CLI

> **Note**: An example Triton cache can be found in the
[tests](../tests/) directory (e.g., `vector-add-cache/`).

We demonstrate how to build a *compat* image with Docker CLI. Produced
images have `application/vnd.docker.image.rootfs.diff.tar.gzip` layer
media type.

We assume that you have a Triton cache that you want to package as an image.

1. First, we prepare the following Dockerfile:

```bash
$ cat Dockerfile
FROM scratch
LABEL org.opencontainers.image.title=01-vector-add-latest
COPY vector-add-cache ./io.triton.cache
```

> NOTE: you must have exactly one `COPY` instruction in the Dockerfile
  at the end as only the last layer in produced images is going to be
  taken into account to obtain the files.

1. Then, build your image via `docker build` command

```bash
docker build -t quay.io/tkm/triton-cache:01-vector-add-latest .
```

1. Finally, push the image to your registry via `docker push` command

```bash
docker push quay.io/tkm/triton-cache:01-vector-add-latest
```

> **Note**: The same can be done for a vLLM cache.
