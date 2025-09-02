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
2. **ConfigMap rollout strategy** - Fallback when `defaultDeploymentMode` is `"Standard"`

## RolloutSpec (ConfigMap Configuration)

Defines the rollout strategy configuration for ConfigMap defaults. Users can configure different rollout modes by setting appropriate `maxSurge` and `maxUnavailable` values:

**Availability Mode (Zero Downtime)**:
- Set `maxUnavailable: "0"` and `maxSurge` to desired value/percentage
- New pods are created before old pods are terminated

**ResourceAware Mode (Resource Efficient)**:
- Set `maxSurge: "0"` and `maxUnavailable` to desired value/percentage  
- Old pods are terminated before new pods are created

### Fields

| Field | Type | Description | Required | Default |
|-------|------|-------------|----------|---------|
| `maxSurge` | `string` | Maximum number of pods that can be created above desired replica count (e.g., `"1"`, `"25%"`) | Yes | - |
| `maxUnavailable` | `string` | Maximum number of pods that can be unavailable during update (e.g., `"1"`, `"25%"`) | Yes | - |



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
      "defaultDeploymentMode": "Standard",
      "rawDeploymentRolloutStrategy": {
        "defaultRollout": {
          "maxSurge": "1",        # For Availability mode: set maxUnavailable: "0" 
          "maxUnavailable": "1"   # For ResourceAware mode: set maxSurge: "0"
        }
      }
    }
```

## Example InferenceService (Direct DeploymentStrategy)

### Availability Mode Example:
```yaml
apiVersion: serving.kserve.io/v1beta1
kind: InferenceService
metadata:
  name: availability-mode-example
  annotations:
    serving.kserve.io/deploymentMode: "Standard"
spec:
  predictor:
    model:
      modelFormat:
        name: sklearn
      storageUri: "s3://my-bucket/model"
    # Availability mode: maxUnavailable = 0, maxSurge = desired value
    deploymentStrategy:
      type: RollingUpdate
      rollingUpdate:
        maxUnavailable: "0"    # Zero downtime
        maxSurge: "1"          # Allow one extra pod
```

### ResourceAware Mode Example:
```yaml
apiVersion: serving.kserve.io/v1beta1
kind: InferenceService
metadata:
  name: resource-aware-example
  annotations:
    serving.kserve.io/deploymentMode: "Standard"
spec:
  predictor:
    model:
      modelFormat:
        name: sklearn
      storageUri: "s3://my-bucket/model"
    # ResourceAware mode: maxSurge = 0, maxUnavailable = desired value
    deploymentStrategy:
      type: RollingUpdate
      rollingUpdate:
        maxSurge: "0"          # Resource efficient
        maxUnavailable: "1"    # Allow one pod unavailable
```

## Example InferenceService (Using ConfigMap Defaults)

```yaml
apiVersion: serving.kserve.io/v1beta1
kind: InferenceService
metadata:
  name: example-configmap-defaults
  annotations:
    serving.kserve.io/deploymentMode: "Standard"
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
1. **maxSurge Validation**: Must be a valid number or percentage string
   - Valid percentages: `"25%"`, `"50%"`, `"100%"`
   - Valid numbers: `"1"`, `"2"`, `"5"`
2. **maxUnavailable Validation**: Same format as maxSurge

### For Direct DeploymentStrategy:
1. **type**: Must be `"RollingUpdate"`
2. **rollingUpdate.maxSurge**: Same validation as ConfigMap maxSurge
3. **rollingUpdate.maxUnavailable**: Same validation as ConfigMap maxUnavailable

## Priority Order

When configuring rollout strategies, the following priority order applies:

1. **Multinode deployment override** (HIGHEST priority) - automatic for Ray workloads with `RAY_NODE_COUNT` environment variable
2. **User-defined deploymentStrategy** (high priority) - specified in component extension spec
3. **ConfigMap rollout strategy** (fallback) - only applies when `defaultDeploymentMode` is `"Standard"`
4. **KServe default values** (if no configuration is provided)

**Important**: The ConfigMap rollout strategy only applies when:
- No user-defined `deploymentStrategy` is specified in the component spec
- The `defaultDeploymentMode` in the ConfigMap is set to `"Standard"`

## Default Values

### KServe Defaults
When no rollout strategy is specified anywhere, KServe applies these defaults:
- **maxUnavailable**: `25%`
- **maxSurge**: `25%`

### Multinode Deployment Override
For multinode deployments (Ray workloads), KServe automatically overrides ALL rollout strategy configurations with:
- **maxUnavailable**: `0%`
- **maxSurge**: `100%`

This override takes precedence over all other configurations, including user-defined `deploymentStrategy`.

### Default Values Summary

| Configuration | maxUnavailable | maxSurge | Notes |
|---------------|----------------|----------|-------|
| **No rollout strategy specified** | `25%` | `25%` | KServe defaults |
| **Multinode deployment** | `0%` | `100%` | Overrides ALL other configurations |
| **Availability mode** | `0` | `<ratio>` | From rollout spec |
| **ResourceAware mode** | `<ratio>` | `0` | From rollout spec | 