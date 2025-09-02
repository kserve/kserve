# Rollout Strategy Guide

## Overview

KServe supports configurable rollout strategies for raw deployments, allowing you to control how new versions of your models are deployed. Rollout strategies can be configured through the ConfigMap defaults or by setting Kubernetes `DeploymentStrategy` directly in the component extension spec.

## Configuration Priority

The rollout strategy is applied with the following precedence:

1. **User-defined DeploymentStrategy** (highest priority) - directly specified in component extension spec
2. **ConfigMap rollout strategy** (fallback) - applies only when `defaultDeploymentMode` is `"RawDeployment"`

## ConfigMap Configuration

When using ConfigMap configuration, you can specify `maxSurge` and `maxUnavailable` values directly. These values are applied to the Kubernetes deployment strategy when `defaultDeploymentMode` is set to `"Standard"`.

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
    serving.kserve.io/deploymentMode: "Standard"
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

Configure defaults in the KServe ConfigMap that apply when no user-defined deployment strategy is specified and `defaultDeploymentMode` is set to `"Standard"`:

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
      "deploymentRolloutStrategy": {
        "defaultRollout": {
          "mode": "Availability",
          "maxSurge": "1",
          "maxUnavailable": "1"
        }
      }
    }
```

## Rollout Strategy Modes

KServe supports two main rollout strategy approaches that you can configure either globally via ConfigMap or per-service via `deploymentStrategy`:

### Availability Mode (Zero Downtime)
- **Purpose**: Ensures high availability during deployments by launching new pods first
- **Configuration**: Set `maxUnavailable: "0"` and `maxSurge` to desired value/percentage
- **Behavior**: New pods are created before old pods are terminated
- **Use Case**: Production environments where downtime is not acceptable

### ResourceAware Mode (Resource Efficient)  
- **Purpose**: Optimizes resource usage during deployments by terminating old pods first
- **Configuration**: Set `maxSurge: "0"` and `maxUnavailable` to desired value/percentage
- **Behavior**: Old pods are terminated before new pods are created
- **Use Case**: Resource-constrained environments or cost optimization

### Configuration Parameters

For both direct `deploymentStrategy` and ConfigMap configuration:
- **maxSurge**: Maximum number of pods that can be created above the desired replica count (e.g., `"1"`, `"25%"`)
- **maxUnavailable**: Maximum number of pods that can be unavailable during update (e.g., `"1"`, `"25%"`)

KServe can configure default `maxSurge` and `maxUnavailable` values globally for all InferenceServices via ConfigMap. When users do not specify anything in the `deploymentStrategy` section of their InferenceService, the service will pick up these default values from the ConfigMap when `defaultDeploymentMode` is `"Standard"`.

For direct DeploymentStrategy configuration:
- **type**: Should be `"RollingUpdate"`
- **rollingUpdate.maxSurge**: Same as above
- **rollingUpdate.maxUnavailable**: Same as above

### KServe Defaults (When No ConfigMap Defaults)

If neither the InferenceService spec nor the ConfigMap defines rollout strategy values, KServe applies its own defaults:

- **maxUnavailable**: `25%`
- **maxSurge**: `25%`

### Special Case: Multinode Deployments

For multinode deployments (Ray workloads), KServe automatically overrides ALL rollout strategy configurations to ensure high availability:

- **maxUnavailable**: `0%` (no pods are taken down during rollout)
- **maxSurge**: `100%` (can have up to double the number of pods during rollout)

**Important**: This override takes precedence over ALL other configurations, including:
- User-defined `deploymentStrategy` in the component spec
- ConfigMap rollout strategy defaults
- KServe default values

This behavior is triggered automatically when the `RAY_NODE_COUNT` environment variable is detected in the inference service configuration. It ensures that original pods remain available until new pods are ready, which is critical for maintaining distributed Ray cluster stability during updates.

### Priority Order

The final rollout strategy values are determined by this priority order:

1. **Multinode deployment override** (HIGHEST priority) - automatic for Ray workloads with `RAY_NODE_COUNT` environment variable
2. **User-defined DeploymentStrategy** (high priority) - specified in component extension spec
3. **ConfigMap rollout strategy** (fallback) - only applies when `defaultDeploymentMode` is `"Standard"`
4. **KServe default values** (if no configuration is provided)

**Important**: The ConfigMap rollout strategy only applies when:
- No user-defined `deploymentStrategy` is specified in the component spec
- The `defaultDeploymentMode` in the ConfigMap is set to `"Standard"`

### Default Values Summary

| Configuration | maxUnavailable | maxSurge | Notes |
|---------------|----------------|----------|-------|
| **No rollout strategy specified** | `25%` | `25%` | KServe defaults |
| **Multinode deployment** | `0%` | `100%` | Overrides ALL other configurations |
| **Availability mode** | `0` | `<ratio>` | From rollout spec |
| **ResourceAware mode** | `<ratio>` | `0` | From rollout spec |

## Examples

### Example 1: Availability Mode - High Availability Deployment

**Availability Mode**: Launch new pods first, terminate old pods after (zero downtime)

```yaml
apiVersion: serving.kserve.io/v1beta1
kind: InferenceService
metadata:
  name: availability-mode-model
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
        maxUnavailable: "0"    # No pods unavailable during rollout
        maxSurge: "100%"       # Can double the pods during rollout
```

**Behavior**: New pods are created first, then old pods are terminated. Ensures zero downtime but uses more resources temporarily.

### Example 2: ResourceAware Mode - Resource-Efficient Deployment

**ResourceAware Mode**: Terminate old pods first, launch new pods after (resource efficient)

```yaml
apiVersion: serving.kserve.io/v1beta1
kind: InferenceService
metadata:
  name: resource-aware-model
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
        maxSurge: "0"          # No extra pods during rollout
        maxUnavailable: "25%"  # Up to 25% of pods can be unavailable
```

**Behavior**: Old pods are terminated first, then new pods are created. Minimizes resource usage but may cause temporary unavailability.

### Example 3: Using ConfigMap Global Defaults

When no `deploymentStrategy` is specified, the InferenceService picks up default values from the KServe ConfigMap:

```yaml
apiVersion: serving.kserve.io/v1beta1
kind: InferenceService
metadata:
  name: configmap-defaults-model
  annotations:
    serving.kserve.io/deploymentMode: "Standard"
spec:
  predictor:
    model:
      modelFormat:
        name: sklearn
      storageUri: "s3://my-bucket/model"
    # No deploymentStrategy specified - will use ConfigMap defaults
    # when defaultDeploymentMode is "Standard"
```

**Behavior**: Uses the global `deploymentRolloutStrategy` configuration from the KServe ConfigMap, allowing administrators to set organization-wide rollout policies.

### Example 4: No Rollout Strategy (Uses KServe Defaults)

```yaml
apiVersion: serving.kserve.io/v1beta1
kind: InferenceService
metadata:
  name: default-rollout-model
  annotations:
    serving.kserve.io/deploymentMode: "Standard"
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
      "defaultDeploymentMode": "Standard",
      "deploymentRolloutStrategy": {
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

1. **ConfigMap not applied**: Ensure the ConfigMap rollout strategy only applies when `defaultDeploymentMode` is `"Standard"`
2. **Invalid Mode**: For ConfigMap configuration, ensure mode is exactly `"Availability"` or `"ResourceAware"` (case-sensitive)
3. **Invalid maxSurge/maxUnavailable**: Ensure values are valid numbers or percentages (e.g., `"1"`, `"25%"`)
4. **Missing Annotation**: Ensure `serving.kserve.io/deploymentMode: "Standard"` is set for raw deployments
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
kubectl get configmap inferenceservice-config -n kserve -o jsonpath='{.data.deploy}' | jq '.deploymentRolloutStrategy'

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