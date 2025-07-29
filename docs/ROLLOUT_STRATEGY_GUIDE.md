# Rollout Strategy Guide

## Overview

KServe supports configurable rollout strategies for raw deployments, allowing you to control how new versions of your models are deployed. This feature provides two distinct rollout modes: **Availability** and **ResourceAware**, each optimized for different deployment scenarios.

## Rollout Modes

### Availability Mode
- **Purpose**: Ensures high availability during deployments
- **Behavior**: Launches new pods first, then terminates old pods
- **Kubernetes Settings**: `maxUnavailable=0`, `maxSurge=<ratio>`
- **Use Case**: Production environments where downtime is not acceptable

### ResourceAware Mode
- **Purpose**: Optimizes resource usage during deployments
- **Behavior**: Terminates old pods first, then launches new pods
- **Kubernetes Settings**: `maxSurge=0`, `maxUnavailable=<ratio>`
- **Use Case**: Resource-constrained environments or cost optimization

## Configuration

### InferenceService Level Configuration

You can configure rollout strategy at the component level (predictor, transformer, explainer) within your InferenceService:

```yaml
apiVersion: serving.kserve.io/v1beta1
kind: InferenceService
metadata:
  name: rollout-strategy-example
  namespace: default
  annotations:
    serving.kserve.io/deploymentMode: "RawDeployment"
spec:
  predictor:
    model:
      modelFormat:
        name: sklearn
      storageUri: "s3://my-bucket/model"
    # Availability mode - high availability
    rollout:
      mode: "Availability"
      ratio: "50%"
    
  transformer:
    custom:
      container:
        image: my-transformer:latest
    # ResourceAware mode - resource efficiency
    rollout:
      mode: "ResourceAware"
      ratio: "25%"
```

### Configuration Parameters

- **mode**: Either `"Availability"` or `"ResourceAware"`
- **ratio**: Can be specified as:
  - Percentage (e.g., `"25%"`, `"50%"`)
  - Absolute number (e.g., `"2"`, `"5"`)

### Default Configuration

### ConfigMap Defaults

If no rollout strategy is specified in the InferenceService, KServe will use default values from the ConfigMap:

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

### KServe Defaults (When No ConfigMap Defaults)

If neither the InferenceService spec nor the ConfigMap defines rollout strategy values, KServe applies its own defaults:

- **maxUnavailable**: `25%`
- **maxSurge**: `25%`

### Special Case: Multinode Deployments

For multinode deployments, KServe automatically overrides the rollout strategy to ensure high availability:

- **maxUnavailable**: `0%` (no pods are taken down during rollout)
- **maxSurge**: `100%` (can have up to double the number of pods during rollout)

This ensures that original pods remain available until new pods are ready.

### Priority Order

The final rollout strategy values are determined by this priority order:

1. **InferenceService spec values** (highest priority)
2. **ConfigMap default values** (fallback)
3. **KServe default values** (if no rollout strategy configured)
4. **Multinode override** (if applicable)

### Default Values Summary

| Configuration | maxUnavailable | maxSurge | Notes |
|---------------|----------------|----------|-------|
| **No rollout strategy specified** | `25%` | `25%` | KServe defaults |
| **Multinode deployment** | `0%` | `100%` | Overrides KServe defaults |
| **Availability mode** | `0` | `<ratio>` | From rollout spec |
| **ResourceAware mode** | `<ratio>` | `0` | From rollout spec |

## Examples

### Example 1: High Availability Deployment

```yaml
apiVersion: serving.kserve.io/v1beta1
kind: InferenceService
metadata:
  name: high-availability-model
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
      ratio: "100%"
```

This configuration ensures that during deployment:
- `maxUnavailable=0` (no pods are taken down during rollout)
- `maxSurge=100%` (can have up to double the number of pods during rollout)

### Example 2: Resource-Efficient Deployment

```yaml
apiVersion: serving.kserve.io/v1beta1
kind: InferenceService
metadata:
  name: resource-efficient-model
  annotations:
    serving.kserve.io/deploymentMode: "RawDeployment"
spec:
  predictor:
    model:
      modelFormat:
        name: sklearn
      storageUri: "s3://my-bucket/model"
    rollout:
      mode: "ResourceAware"
      ratio: "25%"
```

This configuration ensures that during deployment:
- `maxSurge=0` (no extra pods are created during rollout)
- `maxUnavailable=25%` (up to 25% of pods can be unavailable during rollout)

### Example 3: Mixed Strategy Deployment

```yaml
apiVersion: serving.kserve.io/v1beta1
kind: InferenceService
metadata:
  name: mixed-strategy-model
  annotations:
    serving.kserve.io/deploymentMode: "RawDeployment"
spec:
  predictor:
    model:
      modelFormat:
        name: sklearn
      storageUri: "s3://my-bucket/model"
    # High availability for predictor
    rollout:
      mode: "Availability"
      ratio: "50%"
  
  transformer:
    custom:
      container:
        image: my-transformer:latest
    # Resource efficient for transformer
    rollout:
      mode: "ResourceAware"
      ratio: "25%"
```

### Example 4: No Rollout Strategy (Uses KServe Defaults)

```yaml
apiVersion: serving.kserve.io/v1beta1
kind: InferenceService
metadata:
  name: default-rollout-model
  annotations:
    serving.kserve.io/deploymentMode: "RawDeployment"
spec:
  predictor:
    model:
      modelFormat:
        name: sklearn
      storageUri: "s3://my-bucket/model"
    # No rollout strategy specified - will use KServe defaults:
    # maxUnavailable: "25%", maxSurge: "25%"
```

**Note**: For multinode deployments, even if no rollout strategy is specified, KServe will automatically override with `maxUnavailable: "0%"` and `maxSurge: "100%"` to ensure high availability.

## Validation

The rollout strategy configuration is validated to ensure:
- **mode** is one of: `"Availability"` or `"ResourceAware"`
- **ratio** is a valid number or percentage string
- Percentage values must be properly formatted (e.g., `"25%"` not `"25"`)

## Best Practices

1. **Production Environments**: Use `Availability` mode with a ratio of `25%` to `50%` for most production workloads
2. **Resource-Constrained Clusters**: Use `ResourceAware` mode to minimize resource usage during deployments
3. **Critical Services**: Use `Availability` mode with `100%` ratio for zero-downtime deployments
4. **Testing**: Use `ResourceAware` mode with `50%` ratio for development and testing environments

## Migration from Previous Versions

If you were using the previous rollout strategy configuration (individual `rolloutStrategy` and `rolloutRatio` fields), update your InferenceService specs to use the new nested `rollout` object:

```yaml
# Old format (deprecated)
spec:
  predictor:
    rolloutStrategy: "Availability"
    rolloutRatio: "25%"

# New format
spec:
  predictor:
    rollout:
      mode: "Availability"
      ratio: "25%"
```

## Troubleshooting

### Common Issues

1. **Invalid Mode**: Ensure the mode is exactly `"Availability"` or `"ResourceAware"` (case-sensitive)
2. **Invalid Ratio**: Ensure ratio is a valid number or percentage (e.g., `"25%"`, `"2"`)
3. **Missing Annotation**: Ensure `serving.kserve.io/deploymentMode: "RawDeployment"` is set for raw deployments

### Verification

To verify your rollout strategy is working correctly:

```bash
# Check the deployment strategy
kubectl get deployment <deployment-name> -o jsonpath='{.spec.strategy}'

# Check the rollout status
kubectl rollout status deployment <deployment-name>

# Check the pods during rollout
kubectl get pods -l app=<deployment-name>
```

### Checking Default Values

To see what default values are actually being applied:

```bash
# Check if ConfigMap has rollout defaults
kubectl get configmap inferenceservice-config -n kserve -o jsonpath='{.data.deploy}' | jq '.rawDeploymentRolloutStrategy'

# Check the actual deployment strategy being used
kubectl get deployment <deployment-name> -o jsonpath='{.spec.strategy.rollingUpdate}'

# Example output for KServe defaults:
# {
#   "maxUnavailable": "25%",
#   "maxSurge": "25%"
# }
```

### Understanding Which Defaults Are Applied

1. **If you see `maxUnavailable: "0%"` and `maxSurge: "100%"`**: This is a multinode deployment override
2. **If you see `maxUnavailable: "25%"` and `maxSurge: "25%"`**: These are KServe defaults (no rollout strategy specified)
3. **If you see custom values**: These are from your InferenceService spec or ConfigMap defaults

## Related Documentation

- [InferenceService API Reference](../apis/v1beta1/README.md)
- [Raw Deployment Guide](https://github.com/kserve/website/blob/main/docs/modelserving/raw_deployment.md)
- [Canary Rollout Guide](../samples/v1beta1/rollout/README.md) 