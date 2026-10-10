# OCI Image Creation and Delta Capture

MCV supports two image creation paths:

- Docker or Buildah creates a local image using the available container tool.
- The OCI builder packages the cache in-process and pushes it directly to a registry.

The OCI builder is the path used by the KernelCache capture sidecar.

## Create an OCI image

```bash
mcv --create \
  --builder oci \
  --image quay.io/example/kernel-cache:latest \
  --dir /path/to/cache
```

The OCI builder creates a single-layer compat image and pushes it to the registry. The command does not require a local Docker or Buildah daemon.

## Capture only a baseline snapshot

```bash
mcv --snapshot \
  --dir /path/to/cache \
  --snapshot-file /tmp/mcv/cache-snapshot.json
```

The snapshot records directories and regular files below the cache root. File content is identified with a SHA-256 digest. `dummy_cache` is excluded by default. Additional directories can be excluded with repeated `--exclude-dir`
flags:

```bash
mcv --snapshot \
  --dir /path/to/cache \
  --exclude-dir dummy_cache \
  --exclude-dir temporary \
  --snapshot-file /tmp/mcv/cache-snapshot.json
```

## Create an image from changes after a snapshot

```bash
mcv --create \
  --builder oci \
  --image quay.io/example/kernel-cache:capture-1 \
  --dir /path/to/cache \
  --delta-from-snapshot \
  --snapshot-file /tmp/mcv/cache-snapshot.json \
  --result /tmp/mcv/result.json
```

The command compares the saved snapshot with the current cache. If only new or changed content is present below existing cache directories, MCV creates a directory-only OCI image. The image contains the current contents of the affected directories, not the snapshot file itself.

MCV creates a full image when a directory-only layer cannot represent the change. This includes:

- deleted files
- changes to files directly below the cache root
- an initial cache with no usable baseline content
- a legacy snapshot without file state

If there are no changes after applying exclusions, the command returns an `Unchanged` result and does not push an image.

## Result file

Pass `--result` to write a JSON result after image creation:

```json
{
  "state": "Succeeded",
  "imageReference": "quay.io/example/kernel-cache@sha256:...",
  "cacheSizeBytes": 123456,
  "completedAt": "2026-09-16T12:00:00Z"
}
```

The `state` is either `Succeeded` or `Unchanged`. `imageReference` is the
immutable digest reference for a pushed image and is empty for `Unchanged`.

## Registry access

When no explicit credential is configured, MCV uses the default registry keychain. For workload or service-account credentials, configure the registry access before using the OCI builder:

| Variable | Purpose |
|---|---|
| `MCV_REGISTRY_ACCESS_FILE` | JSON access file containing registry, username, token, and expiry |
| `MCV_REGISTRY_ALLOWED_ENDPOINT` | Registry allowed to receive the configured credential |
| `MCV_REGISTRY_CA_FILE` | Additional CA certificate for the registry |

Credentials are checked against the target registry before a request is sent.

## Cache links

The OCI staging path can resolve absolute cache symlinks when `MCV_CACHE_LINK_ROOT` is set. The link target must remain inside that allowed root. Links outside the root and unsupported special files are rejected.

## Snapshot compatibility

The current snapshot format includes file state. Older directory-only snapshots are still readable, but MCV falls back to a full image when they are used for delta creation.
