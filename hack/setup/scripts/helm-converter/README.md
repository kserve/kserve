# Kustomize to Helm Converter

A tool to convert KServe's Kustomize manifests to Helm charts.

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Generated Charts](#generated-charts)
- [Version and Image Tag Management](#version-and-image-tag-management)
- [Mapping Files](#mapping-files)
- [How It Works](#how-it-works)
- [Limitations](#limitations)
- [Notes](#notes)
- [References](#references)
- [License](#license)

## Overview

### Why this tool?

KServe is built with Kubebuilder and managed with Kustomize manifests. However, users often prefer Helm. This tool:

1. ✅ Keeps Kustomize manifests as the source of truth
2. ✅ Exposes only explicitly defined fields as Helm values via mapping files
3. ✅ Generates identical manifests with default values as Kustomize
4. ✅ Provides sustainable and maintainable conversion
5. ✅ Preserves Go templates in resources using copyAsIs mechanism
6. ✅ Includes automated verification against Kustomize output

### Core Principles

- **Source of Truth**: Kustomize manifests are always the source
- **Explicit Mapping**: Only fields defined in mapping files are exposed as values
- **Identity Guarantee**: Default values produce identical results as Kustomize
- **Auto-sync**: By default, overwrites existing charts to stay in sync with Kustomize

## Quick Start

### Generate and Verify Charts

```bash
# Generate and verify all charts at once using Makefile (recommended)
make generate-helm-charts
```

## Generated Charts

### 1. kserve-resources chart

Core KServe controller for traditional ML and deep learning model serving.

**Configuration options:**

- `kserve.controller.image`: Controller image (default: "kserve/kserve-controller")
- `kserve.controller.tag`: Controller image tag
- `kserve.controller.imagePullPolicy`: Image pull policy
- `kserve.controller.resources`: Resource limits/requests
- `inferenceServiceConfig.enabled`: Enable InferenceService ConfigMap
- `certManager.enabled`: Install cert-manager self-signed Issuer

### 2. kserve-llmisvc-resources chart

LLM Inference Service controller for large language models and foundation models.

**Configuration options:**

- `llmisvc.controller.image`: Controller image (default: "kserve/llmisvc-controller")
- `llmisvc.controller.tag`: Controller image tag
- `llmisvc.controller.imagePullPolicy`: Image pull policy
- `llmisvc.controller.resources`: Resource limits/requests
- `inferenceServiceConfig.enabled`: Enable InferenceService ConfigMap
- `certManager.enabled`: Install cert-manager self-signed Issuer

### 3. kserve-localmodel-resources chart

LocalModel controller for local model storage and caching.

**Configuration options:**

- `localmodel.controller.image`: Controller image (default: "kserve/kserve-localmodel-controller")
- `localmodel.controller.tag`: Controller image tag
- `localmodel.controller.imagePullPolicy`: Image pull policy
- `localmodel.controller.resources`: Resource limits/requests
- `localmodel.nodeAgent.image`: Node agent image (default: "kserve/kserve-localmodelnode-agent")
- `localmodel.nodeAgent.tag`: Node agent image tag
- `localmodel.nodeAgent.imagePullPolicy`: Image pull policy
- `localmodel.nodeAgent.resources`: Resource limits/requests

### 4. kserve-runtime-configs chart

ClusterServingRuntimes and LLMInferenceServiceConfigs for KServe.

**Configuration options:**

- `runtimes.enabled`: Install ClusterServingRuntimes (default: true)
  - Individual runtimes: `sklearn`, `xgboost`, `tensorflow`, `triton`, `mlserver`, `pmml`, `lightgbm`, `paddle`, `torchserve`, `huggingface`, `huggingfaceMultinode`, `predictive`
  - Each runtime has: `enabled`, `image.repository`, `image.tag`, `resources`
- `llmisvcConfigs.enabled`: Install LLMInferenceServiceConfigs (default: false)


**Note:**

- LLMInferenceServiceConfig resources contain Go template expressions preserved using `copyAsIs` mechanism
- CRDs are managed separately and not included in charts


## Version and Image Tag Management

### Updating Chart Version

**IMPORTANT:** Chart versions are automatically synced from `KSERVE_VERSION` in `kserve-deps.env`.

The converter automatically reads `KSERVE_VERSION` from `kserve-deps.env` and overrides the version in mapping files. This ensures all charts use the same version as the main project.

To update the chart version for a release:

1. **Update `kserve-deps.env`** (single source of truth):

   ```bash
   # kserve-deps.env
   KSERVE_VERSION=v0.16.0
   ```

2. **Regenerate all charts**:

   ```bash
   # The converter automatically uses KSERVE_VERSION from kserve-deps.env
   make convert-helm-charts
   ```

**Note**
```
# mapper has this to get the KSERVE_VERSION from kserve-deps
globals:
  version:
    valuePath: kserve.version
    kserve-deps: KSERVE_VERSION
```

**Version Flow:**

```text
kserve-deps.env (KSERVE_VERSION=v0.16.0)
    ↓ (auto-read by converter)
Chart.yaml (version: v0.16.0, appVersion: v0.16.0)
```

**Note:** You don't need to manually update mapping files - the converter automatically overrides their version/appVersion with the value from `kserve-deps.env`.

### Overriding KServe Version

All charts use `kserve.version` as the default version for all images. You can override the version at installation time:

```bash
# Install with specific version for all images
helm install kserve charts/kserve-resources \
  --set kserve.version=v0.15.0 \
  -n kserve --create-namespace
```

For fine-grained control of individual image tags or registries, refer to `values.yaml`.

## Mapping Files

### Understanding Mapping Files

Mapping files define which manifest fields should be exposed as Helm values. They act as a bridge between Kustomize manifests and Helm charts.

### Mapping File Structure

Mapping files define how to extract values from Kustomize manifests and expose them as Helm values:

```yaml
# Example: Mapping a deployment image to values.yaml
image:
  repository:
    path: spec.template.spec.containers[0].image+(:,0)  # Extract "kserve/controller"
    valuePath: kserve.controller.image
  tag:
    path: spec.template.spec.containers[0].image+(:,1)  # Extract "v0.16.0"
    valuePath: kserve.controller.tag
```

For detailed mapping syntax, refer to existing mapping files below.

### Available Mapping Files

The converter uses the following mapping files:

- **[helm-mapping-common.yaml](mappers/helm-mapping-common.yaml)**: Shared configurations (ConfigMap, Issuer)
- **[helm-mapping-kserve.yaml](mappers/helm-mapping-kserve.yaml)**: KServe controller and resources
- **[helm-mapping-llmisvc.yaml](mappers/helm-mapping-llmisvc.yaml)**: LLMISVC controller and resources
- **[helm-mapping-localmodel.yaml](mappers/helm-mapping-localmodel.yaml)**: LocalModel controller and node agent
- **[helm-mapping-kserve-runtime-configs.yaml](mappers/helm-mapping-kserve-runtime-configs.yaml)**: ClusterServingRuntimes and LLM configs


## How It Works

After generating Helm charts, the converter automatically verifies them against Kustomize manifests using `compare_manifests.py`:

**Mapper Mismatch Detection:**
- **ConfigMap Nested Fields**: Detects when Helm has nested fields (e.g., `data.localModel.jobTTLSecondsAfterFinished`) that don't exist in Kustomize
- **Deployment/Service Fields**: Detects when mapper defines paths (e.g., `image.repository`, `resources`) that don't exist in Kustomize manifests
- **Actionable Feedback**: Provides clear messages with action items:
  - Add the field to Kustomize manifest, OR
  - Remove the field from mapper configuration

**Difference Classification:**
- **ℹ️ Expected Helm Metadata**: Helm standard labels/annotations added by `_helpers.tpl` (normal, not errors)
  - `helm.sh/chart`, `app.kubernetes.io/managed-by`, `app.kubernetes.io/instance`, etc.
- **❌ Critical Differences**: Fields other than labels/annotations differ (requires action)
  - Includes detailed diff output showing exactly what differs

**Verification Output Example:**
```
✅ PASS: Manifests are equivalent!

ℹ️  Resources with expected Helm metadata differences (3):
   (Helm standard labels/annotations added by _helpers.tpl)
  - Deployment/kserve/kserve-controller-manager
  - Service/kserve/kserve-controller-manager-service
  - Issuer/kserve/selfsigned-issuer
```

**Mapper Mismatch Example:**
```
❌ FAIL: Critical differences found

❌ Resources with CRITICAL differences (1):
  - ConfigMap/kserve/inferenceservice-config:
      Data fields differ:
        - data.localModel:
            ⚠️  MAPPER MISMATCH DETECTED:
                - Field 'data.localModel.jobTTLSecondsAfterFinished' is defined in mapper 'helm-mapping-common'
                  but does NOT exist in Kustomize manifest
                  (Helm value: null)
                  Action needed:
                    • Add 'data.localModel.jobTTLSecondsAfterFinished' to Kustomize ConfigMap, OR
                    • Remove 'jobTTLSecondsAfterFinished' from mapper 'helm-mapping-common'
```

This verification ensures:
1. **Source of Truth Principle**: Kustomize manifests remain authoritative
2. **Mapper Accuracy**: Mapper definitions match actual Kustomize structure
3. **User Guidance**: Clear action items when mismatches occur

## Limitations

Current limitations:

1. **Kustomize Features**: Supports basic resources. Complex kustomize patches/overlays may need manual handling
2. **Namespace Creation**: Use `--create-namespace` flag or create namespace beforehand
3. **Mapper Flexibility**: Mappers allow freely adding/removing field mappings, but new resource files or Kinds require converter source code modifications

## Notes

### CRDs Management

CRDs are managed separately via Makefile and are **not included** in the Helm charts. Always install CRDs before installing any Helm charts:

```bash
# Install CRDs first (required)
helm install kserve charts/kserve-crd
# Then install Helm charts
helm install kserve charts/kserve-resources -n kserve --create-namespace
```

### LLMInferenceServiceConfig Resources

LLMInferenceServiceConfig manifests contain Go template expressions that must be preserved for the KServe controller. The copyAsIs mechanism escapes these expressions so Helm doesn't process them, allowing the controller to evaluate them at runtime.

## References

- [Helm Documentation](https://helm.sh/docs/)
- [Kustomize Documentation](https://kustomize.io/)
- [KServe Documentation](https://kserve.github.io/website/)
- [Kubebuilder Documentation](https://book.kubebuilder.io/)

## License

This tool is part of the KServe project and follows the Apache 2.0 license.
