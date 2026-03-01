# Common Values for KServe Helm Charts

This directory contains shared configuration values for KServe Helm charts to eliminate duplication and centralize common settings.

## Directory Structure

```
charts/_common/
├── README.md                                   # This file
├── common-sections.yaml                        # 16 shared sections (108 lines)
├── kserve-resources-specific.yaml              # kserve-controller specific values (75 lines)
├── kserve-llmisvc-resources-specific.yaml      # llmisvc-controller specific values (98 lines)
├── _common.tpl                                 # Common metadata helpers
├── _resources.tpl                              # Resource helpers
└── _utils.tpl                                  # Utility helpers
```

## Files Description

### YAML Values Files

- **`common-sections.yaml`**
  - Contains 16 common sections shared across all KServe charts
  - Includes: version, agent, router, storage, localmodel, servingruntime, security, etc.
  - Total: 108 lines

- **`kserve-resources-specific.yaml`**
  - KServe controller specific configuration
  - Includes: controller image, resources, rbacProxy, gateway settings, etc.
  - Total: 75 lines

- **`kserve-llmisvc-resources-specific.yaml`**
  - LLM ISVC controller specific configuration
  - Includes: llmisvc.controller settings, commonLabels/Annotations, controller gateway
  - Total: 98 lines

### Template Helpers

- **`_common.tpl`** - Common metadata and label helpers
- **`_resources.tpl`** - Resource-related template functions
- **`_utils.tpl`** - Utility template functions

## ⚠️ IMPORTANT: How to Modify Values

**DO NOT edit `charts/kserve-resources/values.yaml` or `charts/kserve-llmisvc-resources/values.yaml` directly!**

These files are **auto-generated** from the source files in this directory.

### Correct Workflow

1. **Edit source files** in `charts/_common/`:
   ```bash
   # For common sections (shared by all charts)
   vi charts/_common/common-sections.yaml

   # For kserve-resources specific values
   vi charts/_common/kserve-resources-specific.yaml

   # For kserve-llmisvc-resources specific values
   vi charts/_common/kserve-llmisvc-resources-specific.yaml
   ```

2. **Run generation script**:
   ```bash
   ./hack/setup/scripts/generate_chart_manifests.sh
   ```

   This will:
   - Build kustomize manifests
   - Merge common and specific values using `yq`
   - Generate `charts/kserve-resources/values.yaml`
   - Generate `charts/kserve-llmisvc-resources/values.yaml`

3. **Verify generated files**:
   ```bash
   helm lint charts/kserve-resources
   helm lint charts/kserve-llmisvc-resources
   ```

## Merge Strategy

The generation script uses `yq` to deep merge YAML files:

```bash
# For kserve-resources
yq eval-all '. as $item ireduce ({}; . * $item)' \
  common-sections.yaml \
  kserve-resources-specific.yaml \
  > ../kserve-resources/values.yaml

# For kserve-llmisvc-resources
yq eval-all '. as $item ireduce ({}; . * $item)' \
  common-sections.yaml \
  kserve-llmisvc-resources-specific.yaml \
  > ../kserve-llmisvc-resources/values.yaml
```

The specific values override/extend common values when keys overlap.

## Benefits

- **Single source of truth**: Common values managed in one place
- **Consistency**: All charts use identical common configurations
- **Maintainability**: Update common values once, apply to all charts
- **Reduced duplication**: ~485 lines of code eliminated (34% reduction)
- **Automated**: Generation script ensures consistency

## Examples

### Adding a new common field

```yaml
# charts/_common/common-sections.yaml
kserve:
  version: v0.16.0
  newCommonField: value  # Add here
  agent:
    image: kserve/agent
```

Then run: `./hack/setup/scripts/generate_chart_manifests.sh`

### Modifying kserve-controller settings

```yaml
# charts/_common/kserve-resources-specific.yaml
kserve:
  controller:
    image: kserve/kserve-controller
    tag: ""
    resources:
      limits:
        cpu: 200m  # Modify here
```

Then run: `./hack/setup/scripts/generate_chart_manifests.sh`

## Troubleshooting

### Generated values.yaml is missing sections

- Ensure source files in `charts/_common/` are valid YAML
- Run `yq` commands manually to check for errors
- Verify `yq` is installed: `yq --version`

### Values not updating

- Make sure you edited files in `charts/_common/`, not in `charts/*/values.yaml`
- Re-run `./hack/setup/scripts/generate_chart_manifests.sh`
- Check script output for errors

### Merge conflicts

- Check indentation in source YAML files
- Ensure keys don't conflict unintentionally
- Use `yq` to validate YAML syntax
