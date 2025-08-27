# Rollout Strategy API Reference

## Overview

This document describes the API fields for rollout strategy configuration in KServe v1beta1. Rollout strategies can be configured through ConfigMap defaults or directly using Kubernetes `DeploymentStrategy`.

## ComponentExtensionSpec

The `ComponentExtensionSpec` supports two approaches for rollout strategy configuration:

### Fields

| Field | Type | Description | Required |
|-------|------|-------------|----------|
| `deploymentStrategy` | `appsv1.DeploymentStrategy` | Direct Kubernetes deployment strategy (highest priority) | No |

### Configuration Priority

1. **deploymentStrategy** - User-defined Kubernetes deployment strategy (highest priority)
2. **ConfigMap rollout strategy** - Fallback when `defaultDeploymentMode` is `"RawDeployment"`

## RolloutSpec (ConfigMap Configuration)

Defines the rollout strategy configuration for ConfigMap defaults.

### Fields

| Field | Type | Description | Required | Default |
|-------|------|-------------|----------|---------|
| `mode` | `string` | Rollout strategy mode. Valid values: `"Availability"`, `"ResourceAware"` | Yes | - |
| `maxSurge` | `string` | Maximum number of pods that can be created above desired replica count (e.g., `"1"`, `"25%"`) | Yes | - |
| `maxUnavailable` | `string` | Maximum number of pods that can be unavailable during update (e.g., `"1"`, `"25%"`) | Yes | - |

### Mode Values

- **`Availability`**: Prioritizes service availability during rollouts
  - Sets `maxUnavailable=0`, `maxSurge=<configured value>`
- **`ResourceAware`**: Prioritizes resource efficiency during rollouts
  - Sets `maxSurge=0`, `maxUnavailable=<configured value>`

## DeployConfig

The `DeployConfig` includes configuration for default rollout strategies.

### Fields

| Field | Type | Description | Required |
|-------|------|-------------|----------|
| `rawDeploymentRolloutStrategy` | `RawDeploymentRolloutStrategy` | Default rollout strategy for raw deployments | No |

## RawDeploymentRolloutStrategy

Defines the default rollout strategy configuration for raw deployments.

### Fields

| Field | Type | Description | Required |
|-------|------|-------------|----------|
| `defaultRollout` | `RolloutSpec` | Default rollout configuration | No |

## Example ConfigMap

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: inferenceservice-config
  namespace: kserve
data:
  deploy: |-
    {
      "defaultDeploymentMode": "RawDeployment",
      "rawDeploymentRolloutStrategy": {
        "defaultRollout": {
          "mode": "Availability",
          "maxSurge": "1",
          "maxUnavailable": "1"
        }
      }
    }
```

## Example InferenceService (Direct DeploymentStrategy)

```yaml
apiVersion: serving.kserve.io/v1beta1
kind: InferenceService
metadata:
  name: example
  annotations:
    serving.kserve.io/deploymentMode: "RawDeployment"
spec:
  predictor:
    model:
      modelFormat:
        name: sklearn
      storageUri: "s3://my-bucket/model"
    deploymentStrategy:
      type: RollingUpdate
      rollingUpdate:
        maxUnavailable: "0"
        maxSurge: "1"
```

## Example InferenceService (Using ConfigMap Defaults)

```yaml
apiVersion: serving.kserve.io/v1beta1
kind: InferenceService
metadata:
  name: example-configmap-defaults
  annotations:
    serving.kserve.io/deploymentMode: "RawDeployment"
spec:
  predictor:
    model:
      modelFormat:
        name: sklearn
      storageUri: "s3://my-bucket/model"
    # No deploymentStrategy specified - uses ConfigMap defaults
```

## Validation Rules

### For ConfigMap Configuration:
1. **Mode Validation**: Must be exactly `"Availability"` or `"ResourceAware"` (case-sensitive)
2. **maxSurge Validation**: Must be a valid number or percentage string
   - Valid percentages: `"25%"`, `"50%"`, `"100%"`
   - Valid numbers: `"1"`, `"2"`, `"5"`
3. **maxUnavailable Validation**: Same format as maxSurge

### For Direct DeploymentStrategy:
1. **type**: Must be `"RollingUpdate"`
2. **rollingUpdate.maxSurge**: Same validation as ConfigMap maxSurge
3. **rollingUpdate.maxUnavailable**: Same validation as ConfigMap maxUnavailable

## Priority Order

When configuring rollout strategies, the following priority order applies:

1. **User-defined deploymentStrategy** (highest priority) - specified in component extension spec
2. **ConfigMap rollout strategy** (fallback) - only applies when `defaultDeploymentMode` is `"RawDeployment"`
3. **KServe default values** (if no configuration is provided)

**Important**: The ConfigMap rollout strategy only applies when:
- No user-defined `deploymentStrategy` is specified in the component spec
- The `defaultDeploymentMode` in the ConfigMap is set to `"RawDeployment"`

## Default Values

### KServe Defaults
When no rollout strategy is specified anywhere, KServe applies these defaults:
- **maxUnavailable**: `25%`
- **maxSurge**: `25%`

### Multinode Deployment Override
For multinode deployments, KServe automatically overrides with:
- **maxUnavailable**: `0%`
- **maxSurge**: `100%`

### Default Values Summary

| Configuration | maxUnavailable | maxSurge | Notes |
|---------------|----------------|----------|-------|
| **No rollout strategy specified** | `25%` | `25%` | KServe defaults |
| **Multinode deployment** | `0%` | `100%` | Overrides KServe defaults |
| **Availability mode** | `0` | `<ratio>` | From rollout spec |
| **ResourceAware mode** | `<ratio>` | `0` | From rollout spec | 