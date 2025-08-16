# Rollout Strategy API Reference

## Overview

This document describes the API fields for the rollout strategy feature in KServe v1beta1.

## ComponentExtensionSpec

The `ComponentExtensionSpec` includes a new optional `rollout` field for configuring rollout strategies.

### Fields

| Field | Type | Description | Required |
|-------|------|-------------|----------|
| `rollout` | `RolloutSpec` | Rollout strategy configuration for raw deployments | No |

## RolloutSpec

Defines the rollout strategy configuration for raw deployments.

### Fields

| Field | Type | Description | Required | Default |
|-------|------|-------------|----------|---------|
| `mode` | `string` | Rollout strategy mode. Valid values: `"Availability"`, `"ResourceAware"` | Yes | - |
| `ratio` | `string` | Rollout ratio as percentage (e.g., `"25%"`) or absolute number (e.g., `"2"`) | Yes | - |

### Mode Values

- **`Availability`**: Launches new pods first, then terminates old pods
  - Sets `maxUnavailable=0`, `maxSurge=<ratio>`
- **`ResourceAware`**: Terminates old pods first, then launches new pods
  - Sets `maxSurge=0`, `maxUnavailable=<ratio>`

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
      "defaultDeploymentMode": "Serverless",
      "rawDeploymentRolloutStrategy": {
        "defaultRollout": {
          "mode": "Availability",
          "ratio": "25%"
        }
      }
    }
```

## Example InferenceService

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
    rollout:
      mode: "Availability"
      ratio: "50%"
```

## Validation Rules

1. **Mode Validation**: Must be exactly `"Availability"` or `"ResourceAware"` (case-sensitive)
2. **Ratio Validation**: Must be a valid number or percentage string
   - Valid percentages: `"25%"`, `"50%"`, `"100%"`
   - Valid numbers: `"1"`, `"2"`, `"5"`
   - Invalid: `"25"` (missing %), `"invalid"`, `""`

## Priority Order

When configuring rollout strategies, the following priority order applies:

1. **InferenceService spec values** (highest priority for single deployments)
2. **ConfigMap default values** (fallback for single deployments)
3. **KServe default values** (if no rollout strategy configured for single deployments)
4. **Multinode override** (always applies to worker deployments, overriding all other settings)

**Important**: For multinode deployments, the multinode override (maxUnavailable: 0%, maxSurge: 100%) is applied to worker deployments **after** any rollout strategy is set, effectively overriding spec values and ConfigMap defaults for worker nodes.

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