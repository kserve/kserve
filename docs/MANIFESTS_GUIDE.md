# Helm Chart Developer Guide

This document explains how KServe Helm charts are generated, how the automation works, and what to do when manifests change.

## Architecture Overview

KServe Helm charts use a **Base + Patch** architecture where:

- **Kustomize is the source of truth** for all Kubernetes manifests
- `make generate-chart-manifests` copies `kustomize build` output into `files/` directories as base resources
- Patch files selectively override fields that need to be configurable via `helm --set`
- At render time, Helm deep-merges base + patch to produce the final manifests

![Kustomize to Helm Chart Mapping](images/manifests_compare_helm_kustomize.png)

### Why Kustomize Is the Source of Truth

KServe has always used kustomize manifests in `config/` for non-Helm deployments (`kubectl apply -k`). The Helm charts must produce identical resources.  Rather than maintaining two separate manifest sets that drift apart, the automation copies the `kustomize build` output directly into the chart. This guarantees:

1. A single place to update resource definitions (`config/` directory)
2. Helm and kustomize deployments always stay in sync
3. New resources added to kustomize automatically appear in Helm charts after running `make generate-chart-manifests`

## What `make generate-chart-manifests` Does

Running `make generate-chart-manifests` executes `hack/setup/scripts/generate_chart_manifests.sh`, which performs these steps:

### Step 1: Build kustomize manifests and copy to charts

```bash
# KServe controller resources
kustomize build config/components/kserve > charts/kserve-resources/files/kserve/resources.yaml
kustomize build config/certmanager       > charts/kserve-resources/files/common/certmanager.yaml
kustomize build config/configmap         > charts/kserve-resources/files/common/configmap.yaml
kustomize build config/storagecontainers > charts/kserve-resources/files/common/storagecontainer.yaml

# ClusterServingRuntimes and LLMIsvcConfigs
kustomize build config/llmisvcconfig > charts/kserve-runtime-configs/files/llmisvcconfigs/resources.yaml
kustomize build config/runtimes      > charts/kserve-runtime-configs/files/runtimes/resources.yaml

# LLMISVC controller resources (+ shared resources identical to kserve-resources)
kustomize build config/llmisvc           > charts/kserve-llmisvc-resources/files/llmisvc/resources.yaml
kustomize build config/certmanager       > charts/kserve-llmisvc-resources/files/common/certmanager.yaml
kustomize build config/configmap         > charts/kserve-llmisvc-resources/files/common/configmap.yaml
kustomize build config/storagecontainers > charts/kserve-llmisvc-resources/files/common/storagecontainer.yaml

# LocalModel resources
kustomize build config/localmodels       > charts/kserve-localmodel-resources/files/resources.yaml
```

### Step 2: Generate `values.yaml` from common sections

```bash
# Merge shared values + chart-specific values
yq eval-all '. as $item ireduce ({}; . * $item)' \
  charts/_common/common-sections.yaml \
  charts/_common/kserve-resources-specific.yaml \
  > charts/kserve-resources/values.yaml
```

This is done for `kserve-resources`, `kserve-llmisvc-resources`, and `kserve-localmodel-resources`.

### Step 3: Sync shared patch files

Common patch files (`configmap-patch.yaml`, `storagecontainer-patch.yaml`) are copied from `charts/_common/common-patches/` to `kserve-resources` and `kserve-llmisvc-resources` charts.

Template helpers (`_utils.tpl`, `_common.tpl`, `_resources.tpl`) are synced by separate Makefile targets (`sync-helm-common-helpers`, `sync-helm-common-resource-helpers`, `sync-helm-multi-resource-helpers`), which run as part of `make precommit`.

### Step 4: Lint

```bash
make lint-helm-charts
make verify-helm-helpers-consistency
```

## How Base + Patch Deep Merge Works

At `helm template` time, the template logic in each chart loads the base file and patch file, then deep-merges them:

![Base + Patch Deep Merge](images/manifests_deepMerge.png)

The deep merge (defined in `_utils.tpl`) works recursively:

- **Dicts** are merged recursively (patch keys override base keys, base-only keys are preserved)
- **Arrays with named elements** (containers, env, volumeMounts) are merged by matching the `name` field
- **Other arrays** in patch replace the base array entirely

## How to Handle Changes

### Case 1: New manifest added or existing manifest content changed in kustomize

**What to do:** Just run `make generate-chart-manifests`.

The base `resources.yaml` files are regenerated from `kustomize build` output, so any changes to manifests in `config/` are automatically picked up. No patch file changes are needed unless you want the new/changed fields to be configurable via Helm values.

Examples:

- Adding a new ClusterServingRuntime in `config/runtimes/`
- Changing default resource limits in a runtime YAML
- Adding new env vars or volume mounts
- Updating image tags in `config/runtimes/kustomization.yaml`

### Case 2: Making a field configurable via `helm --set`

If you want users to be able to override a field with `helm install --set`, you need to add it to the appropriate **patch file** and **values.yaml**.

#### For ClusterServingRuntimes

Edit `charts/kserve-runtime-configs/files/runtimes/clusterservingruntimes-patch.yaml`.

This file contains one manifest per runtime with only the fields that reference `{{ .Values.xxx }}`. The patch is matched to the base by `metadata.name` and deep-merged.

Example - adding `imagePullPolicy` as a configurable field for sklearnserver:

```yaml
# In clusterservingruntimes-patch.yaml, find the kserve-sklearnserver section:
apiVersion: serving.kserve.io/v1alpha1
kind: ClusterServingRuntime
metadata:
  name: kserve-sklearnserver
spec:
  disabled: {{ .Values.kserve.servingruntime.sklearnserver.disabled }}
  containers:
    - name: kserve-container
      image: "{{ .Values.kserve.servingruntime.sklearnserver.image }}:{{ ... }}"
      imagePullPolicy: {{ .Values.kserve.servingruntime.sklearnserver.imagePullPolicy }}  # add this
```

Then add the default value in `charts/kserve-runtime-configs/values.yaml`:

```yaml
kserve:
  servingruntime:
    sklearnserver:
      imagePullPolicy: IfNotPresent  # add this
```

#### For ConfigMap (`inferenceservice-config`)

Edit `charts/_common/common-patches/configmap-patch.yaml`.

This patch file is a ConfigMap that contains the entire `inferenceservice-config` data with `{{ .Values.xxx }}` references. It is shared across `kserve-resources` and `kserve-llmisvc-resources` charts.

Example - adding a new field to the `storageInitializer` section:

```yaml
# In configmap-patch.yaml, find the storageInitializer section:
  storageInitializer: |-
    {
        "image" : "{{ .Values.kserve.storage.image }}:{{ ... }}",
        ...
        "newField": "{{ .Values.kserve.storage.newField }}"
    }
```

Then add the default in `charts/_common/common-sections.yaml`:

```yaml
kserve:
  storage:
    newField: defaultValue
```

After editing, run:

```bash
make generate-chart-manifests
```

This copies `configmap-patch.yaml` to both `kserve-resources` and `kserve-llmisvc-resources` charts and regenerates their `values.yaml` files.

#### For ClusterStorageContainer

Edit `charts/_common/common-patches/storagecontainer-patch.yaml`.

This patch overrides image and resource fields for the `ClusterStorageContainer/default` resource. Like `configmap-patch.yaml`, it is shared across charts.

Example:

```yaml
# storagecontainer-patch.yaml
apiVersion: "serving.kserve.io/v1alpha1"
kind: ClusterStorageContainer
metadata:
  name: default
spec:
  container:
    image: "{{ .Values.kserve.storage.image }}:{{ .Values.kserve.storage.tag | default .Values.kserve.version }}"
    resources:
      {{- with .Values.kserve.storage.resources }}
      {{- toYaml . | nindent 6 }}
      {{- end }}
```

#### For controller Deployments, Services, and more

Each component chart has its own patch files under `files/<component>/*-patch.yaml`:

- `kserve-resources`: `files/kserve/deployment-patch.yaml`, `service-patch.yaml`, etc.
- `kserve-llmisvc-resources`: `files/llmisvc/deployment-patch.yaml`, `service-patch.yaml`, etc.
- `kserve-localmodel-resources`: `files/deployment-patch.yaml`, `daemonset-patch.yaml`

These are NOT shared via `_common/` - they are chart-specific because each controller has different configuration needs.

### Case 3: Changing inferenceservice-config ConfigMap defaults

Edit `charts/_common/common-sections.yaml` for values shared across all charts:

```yaml
# charts/_common/common-sections.yaml
kserve:
  agent:
    image: kserve/agent     # agent/batcher/logger image
  router:
    image: kserve/router     # router image
  storage:
    image: kserve/storage-initializer
    enableModelcar: true     # change defaults here
  metricsaggregator:
    enableMetricAggregation: 'false'
  security:
    autoMountServiceAccountToken: true
  # ... all sections referenced by configmap-patch.yaml
```

These values are referenced by `configmap-patch.yaml` via `{{ .Values.kserve.xxx }}` and rendered into the `inferenceservice-config` ConfigMap.

After editing, run:

```bash
make generate-chart-manifests
```

### Case 4: Changing component-specific values

Each chart has its own specific values file in `charts/_common/`:

| File | Chart | What it configures |
| ---- | ----- | ------------------ |
| `kserve-resources-specific.yaml` | `kserve-resources` | KServe controller image, resources, gateway, RBAC proxy, etc. |
| `kserve-llmisvc-resources-specific.yaml` | `kserve-llmisvc-resources` | LLMISVC controller image, resources, probes, replicas, etc. |
| `kserve-localmodel-resources-specific.yaml` | `kserve-localmodel-resources` | LocalModel controller and node agent images, resources, etc. |

Example - changing the default KServe controller resource limits:

```yaml
# charts/_common/kserve-resources-specific.yaml
kserve:
  controller:
    image: kserve/kserve-controller
    resources:
      limits:
        cpu: 200m    # change here
        memory: 500Mi
```

**DO NOT edit `charts/<chart>/values.yaml` directly** - it will be overwritten by `make generate-chart-manifests`.

After editing, run:

```bash
make generate-chart-manifests
```

## Quick Reference

| What changed | What to edit | Then run |
| --- | --- | --- |
| Runtime manifest content (args, env, probes, etc.) | `config/runtimes/kserve-*.yaml` | `make generate-chart-manifests` |
| Runtime image version | `config/runtimes/kustomization.yaml` | `make generate-chart-manifests` |
| Add new ClusterServingRuntime | Add YAML in `config/runtimes/` + update `kustomization.yaml` | `make generate-chart-manifests` |
| Make a runtime field configurable via `--set` | `charts/kserve-runtime-configs/files/runtimes/clusterservingruntimes-patch.yaml` + `values.yaml` | `make lint-helm-charts` |
| ConfigMap (`inferenceservice-config`) structure | `charts/_common/common-patches/configmap-patch.yaml` | `make generate-chart-manifests` |
| ConfigMap default values | `charts/_common/common-sections.yaml` | `make generate-chart-manifests` |
| ClusterStorageContainer config | `charts/_common/common-patches/storagecontainer-patch.yaml` | `make generate-chart-manifests` |
| KServe controller settings | `charts/_common/kserve-resources-specific.yaml` | `make generate-chart-manifests` |
| LLMISVC controller settings | `charts/_common/kserve-llmisvc-resources-specific.yaml` | `make generate-chart-manifests` |
| LocalModel controller settings | `charts/_common/kserve-localmodel-resources-specific.yaml` | `make generate-chart-manifests` |
| Controller deployment patches | `charts/<chart>/files/<component>/*-patch.yaml` | `make lint-helm-charts` |

## File Layout Summary

```text
charts/
├── _common/                          # Shared sources (DO NOT skip this directory)
│   ├── common-sections.yaml          # Shared values across all charts
│   ├── kserve-resources-specific.yaml
│   ├── kserve-llmisvc-resources-specific.yaml
│   ├── kserve-localmodel-resources-specific.yaml
│   ├── common-patches/
│   │   ├── configmap-patch.yaml      # inferenceservice-config patch (shared)
│   │   └── storagecontainer-patch.yaml
│   ├── _utils.tpl                    # deepMerge, replaceNamespace helpers
│   ├── _common.tpl                   # renderSimpleResource, renderPatchedResource
│   └── _resources.tpl               # renderMultiResourceWithPatches
│
├── kserve-runtime-configs/           # ClusterServingRuntimes + LLMIsvcConfigs
│   ├── files/
│   │   ├── runtimes/
│   │   │   ├── resources.yaml                    # AUTO-GENERATED from kustomize build
│   │   │   └── clusterservingruntimes-patch.yaml # Manual: Values overrides
│   │   └── llmisvcconfigs/
│   │       └── resources.yaml                    # AUTO-GENERATED (no patch needed)
│   ├── templates/                    # Helm render logic
│   └── values.yaml                   # Manual: runtime-specific values
│
├── kserve-resources/                 # KServe controller + shared resources
│   ├── files/
│   │   ├── kserve/
│   │   │   ├── resources.yaml        # AUTO-GENERATED from kustomize build
│   │   │   ├── deployment-patch.yaml # Manual: controller deployment overrides
│   │   │   ├── service-patch.yaml
│   │   │   └── ...
│   │   └── common/
│   │       ├── configmap.yaml        # AUTO-GENERATED
│   │       ├── configmap-patch.yaml  # SYNCED from _common/common-patches/
│   │       ├── storagecontainer.yaml # AUTO-GENERATED
│   │       └── storagecontainer-patch.yaml  # SYNCED from _common/common-patches/
│   ├── templates/
│   └── values.yaml                   # AUTO-GENERATED from _common/ sources
│
├── kserve-llmisvc-resources/         # LLMISVC controller + shared resources
│   └── (same structure as kserve-resources)
│
└── kserve-localmodel-resources/      # LocalModel controller
    ├── files/
    │   ├── resources.yaml            # AUTO-GENERATED from kustomize build
    │   ├── deployment-patch.yaml     # Manual: controller deployment overrides
    │   └── daemonset-patch.yaml      # Manual: node agent overrides
    ├── templates/
    └── values.yaml                   # AUTO-GENERATED from _common/ sources
```

**AUTO-GENERATED** files are overwritten by `make generate-chart-manifests`. Do not edit them directly.
