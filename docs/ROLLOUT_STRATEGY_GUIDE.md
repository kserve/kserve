# Rollout Strategy Guide

## Overview

KServe supports configurable rollout strategies for raw deployments, allowing you to control how new versions of your models are deployed. Rollout strategies can be configured through the ConfigMap defaults or by setting Kubernetes `DeploymentStrategy` directly in the component extension spec.

## Configuration Priority

The rollout strategy is applied with the following precedence:

1. **User-defined DeploymentStrategy** (highest priority) - directly specified in component extension spec
2. **ConfigMap rollout strategy** (fallback) - applies only when `defaultDeploymentMode` is `"RawDeployment"`

## Rollout Modes (ConfigMap Configuration)

When using ConfigMap configuration, KServe provides two rollout modes:

### Availability Mode
- **Purpose**: Ensures high availability during deployments
- **Behavior**: Prioritizes service availability during rollouts
- **Kubernetes Settings**: `maxUnavailable=0`, `maxSurge=<configured value>`
- **Use Case**: Production environments where downtime is not acceptable

### ResourceAware Mode
- **Purpose**: Optimizes resource usage during deployments
- **Behavior**: Prioritizes resource efficiency during rollouts
- **Kubernetes Settings**: `maxSurge=0`, `maxUnavailable=<configured value>`
- **Use Case**: Resource-constrained environments or cost optimization

## Configuration

### Method 1: Direct DeploymentStrategy Configuration (Recommended)

You can configure Kubernetes deployment strategy directly at the component level:

```yaml
apiVersion: serving.kserve.io/v1beta1
kind: InferenceService
metadata:
  name: deployment-strategy-example
  namespace: default
  annotations:
    serving.kserve.io/deploymentMode: "RawDeployment"
spec:
  predictor:
    model:
      modelFormat:
        name: sklearn
      storageUri: "s3://my-bucket/model"
    # Direct Kubernetes deployment strategy configuration
    deploymentStrategy:
      type: RollingUpdate
      rollingUpdate:
        maxUnavailable: "0"      # High availability
        maxSurge: "1"            # Allow one extra pod
    
  transformer:
    custom:
      container:
        image: my-transformer:latest
    # Resource-efficient deployment strategy
    deploymentStrategy:
      type: RollingUpdate
      rollingUpdate:
        maxSurge: "0"            # Resource efficient
        maxUnavailable: "1"      # Allow one pod to be unavailable
```

### Method 2: ConfigMap Default Configuration

Configure defaults in the KServe ConfigMap that apply when no user-defined deployment strategy is specified and `defaultDeploymentMode` is set to `"RawDeployment"`:

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

### Configuration Parameters

For ConfigMap configuration:
- **mode**: Either `"Availability"` or `"ResourceAware"`
- **maxSurge**: Maximum number of pods that can be created above the desired replica count (e.g., `"1"`, `"25%"`)
- **maxUnavailable**: Maximum number of pods that can be unavailable during update (e.g., `"1"`, `"25%"`)

For direct DeploymentStrategy configuration:
- **type**: Should be `"RollingUpdate"`
- **rollingUpdate.maxSurge**: Same as above
- **rollingUpdate.maxUnavailable**: Same as above

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

1. **User-defined DeploymentStrategy** (highest priority) - specified in component extension spec
2. **ConfigMap rollout strategy** (fallback) - only applies when `defaultDeploymentMode` is `"RawDeployment"`
3. **KServe default values** (if no configuration is provided)

**Important**: The ConfigMap rollout strategy only applies when:
- No user-defined `deploymentStrategy` is specified in the component spec
- The `defaultDeploymentMode` in the ConfigMap is set to `"RawDeployment"`

### Default Values Summary

| Configuration | maxUnavailable | maxSurge | Notes |
|---------------|----------------|----------|-------|
| **No rollout strategy specified** | `25%` | `25%` | KServe defaults |
| **Multinode deployment** | `0%` | `100%` | Overrides KServe defaults |
| **Availability mode** | `0` | `<ratio>` | From rollout spec |
| **ResourceAware mode** | `<ratio>` | `0` | From rollout spec |

## Examples

### Example 1: High Availability Deployment (Using DeploymentStrategy)

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
    deploymentStrategy:
      type: RollingUpdate
      rollingUpdate:
        maxUnavailable: "0"    # No pods unavailable during rollout
        maxSurge: "100%"       # Can double the pods during rollout
```

This configuration ensures zero downtime during deployments.

### Example 2: Resource-Efficient Deployment (Using DeploymentStrategy)

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
    deploymentStrategy:
      type: RollingUpdate
      rollingUpdate:
        maxSurge: "0"          # No extra pods during rollout
        maxUnavailable: "25%"  # Up to 25% of pods can be unavailable
```

This configuration prioritizes resource efficiency over availability.

### Example 3: ConfigMap Default Configuration

```yaml
apiVersion: serving.kserve.io/v1beta1
kind: InferenceService
metadata:
  name: configmap-defaults-model
  annotations:
    serving.kserve.io/deploymentMode: "RawDeployment"
spec:
  predictor:
    model:
      modelFormat:
        name: sklearn
      storageUri: "s3://my-bucket/model"
    # No deploymentStrategy specified - will use ConfigMap defaults
    # when defaultDeploymentMode is "RawDeployment"
```

This will use the rollout strategy configured in the ConfigMap.

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

### For ConfigMap Configuration:
- **mode** is one of: `"Availability"` or `"ResourceAware"`
- **maxSurge** is a valid number or percentage string (e.g., `"1"`, `"25%"`)
- **maxUnavailable** is a valid number or percentage string (e.g., `"1"`, `"25%"`)

### For Direct DeploymentStrategy:
- **type** must be `"RollingUpdate"`
- **rollingUpdate.maxSurge** follows Kubernetes validation rules
- **rollingUpdate.maxUnavailable** follows Kubernetes validation rules

## Best Practices

1. **Production Environments**: Use `Availability` mode with a ratio of `25%` to `50%` for most production workloads
2. **Resource-Constrained Clusters**: Use `ResourceAware` mode to minimize resource usage during deployments
3. **Critical Services**: Use `Availability` mode with `100%` ratio for zero-downtime deployments
4. **Testing**: Use `ResourceAware` mode with `50%` ratio for development and testing environments

## Migration from Previous Versions

If you were using the previous rollout strategy configuration, update your InferenceService specs to use the new approaches:

### Option 1: Use Direct DeploymentStrategy (Recommended)
```yaml
# Old format (deprecated) 
spec:
  predictor:
    rollout:
      mode: "Availability"
      ratio: "25%"

# New format - Direct DeploymentStrategy
spec:
  predictor:
    deploymentStrategy:
      type: RollingUpdate
      rollingUpdate:
        maxUnavailable: "0"    # Availability mode
        maxSurge: "25%"        # Use the ratio value
```

### Option 2: Use ConfigMap Configuration
Configure defaults in the ConfigMap instead of per-InferenceService configuration:

```yaml
# ConfigMap configuration
data:
  deploy: |-
    {
      "defaultDeploymentMode": "RawDeployment",
      "rawDeploymentRolloutStrategy": {
        "defaultRollout": {
          "mode": "Availability",
          "maxSurge": "25%",
          "maxUnavailable": "25%"
        }
      }
    }

# InferenceService (no deployment strategy needed)
spec:
  predictor:
    model:
      # ... model configuration
    # No rollout configuration - uses ConfigMap defaults
```

## Troubleshooting

### Common Issues

1. **ConfigMap not applied**: Ensure the ConfigMap rollout strategy only applies when `defaultDeploymentMode` is `"RawDeployment"`
2. **Invalid Mode**: For ConfigMap configuration, ensure mode is exactly `"Availability"` or `"ResourceAware"` (case-sensitive)
3. **Invalid maxSurge/maxUnavailable**: Ensure values are valid numbers or percentages (e.g., `"1"`, `"25%"`)
4. **Missing Annotation**: Ensure `serving.kserve.io/deploymentMode: "RawDeployment"` is set for raw deployments
5. **User strategy not taking precedence**: Remember that user-defined `deploymentStrategy` always takes precedence over ConfigMap settings

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

### Checking Configuration

To see what rollout strategy configuration is being applied:

```bash
# Check ConfigMap rollout defaults
kubectl get configmap inferenceservice-config -n kserve -o jsonpath='{.data.deploy}' | jq '.rawDeploymentRolloutStrategy'

# Check the actual deployment strategy being used
kubectl get deployment <deployment-name> -o jsonpath='{.spec.strategy.rollingUpdate}'

# Check if user-defined deploymentStrategy is specified
kubectl get isvc <isvc-name> -o jsonpath='{.spec.predictor.deploymentStrategy}'
```

### Understanding Which Strategy Is Applied

1. **If you see custom maxSurge/maxUnavailable values**: Check if they match your user-defined `deploymentStrategy` (highest priority)
2. **If ConfigMap mode is "Availability"**: Expect `maxUnavailable: "0"` and `maxSurge: <configured value>`
3. **If ConfigMap mode is "ResourceAware"**: Expect `maxSurge: "0"` and `maxUnavailable: <configured value>`
4. **If you see `maxUnavailable: "25%"` and `maxSurge: "25%"`**: These are KServe defaults (no strategy configured anywhere)

## Related Documentation

- [InferenceService API Reference](../apis/v1beta1/README.md)
- [Raw Deployment Guide](https://github.com/kserve/website/blob/main/docs/modelserving/raw_deployment.md)
- [Canary Rollout Guide](../samples/v1beta1/rollout/README.md) 