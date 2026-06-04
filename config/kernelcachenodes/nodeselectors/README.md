# KernelCache Node Selectors

Pre-made nodeSelector examples for KernelCache agent DaemonSet.

This directory contains example patches for common node selection scenarios.
Copy the patch content into your own Kustomize overlay to use.

## Available Examples

- **gpu-nvidia.yaml** - Run agent only on nodes with NVIDIA GPUs (`nvidia.com/gpu: "true"`)
- **gpu-amd.yaml** - Run agent only on nodes with AMD GPUs (`amd.com/gpu: "true"`)
- **kernelcache.yaml** - Run agent only on nodes labeled `kserve/kernelcache: worker`

## Default Behavior

By default (no nodeSelector), the agent DaemonSet runs on **all nodes** in the cluster.
This is the most flexible configuration and works for:
- Heterogeneous clusters (mixed CPU/GPU nodes)
- Testing environments
- Clusters where all nodes should cache kernels

Apply a nodeSelector only when you want to restrict the agent to specific node types.

## Usage

### Option 1: Kustomize Overlay (Recommended for Production)

Create your own overlay with an inline patch:

```yaml
# my-overlay/kustomization.yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
- ../../config/kernelcachenodes  # or path to kernelcachenodes config

patches:
- target:
    kind: DaemonSet
    name: kserve-kernelcachenode-agent
  patch: |-
    apiVersion: apps/v1
    kind: DaemonSet
    metadata:
      name: kserve-kernelcachenode-agent
    spec:
      template:
        spec:
          nodeSelector:
            nvidia.com/gpu: "true"
```

Deploy:
```bash
kubectl apply -k my-overlay/
```

### Option 2: Direct kubectl Patch (Quick Testing)

```bash
# Deploy base config (runs on all nodes)
make deploy-kernelcache

# Apply nodeSelector patch
kubectl patch daemonset kserve-kernelcachenode-agent -n kserve --type strategic \
  --patch-file config/kernelcachenodes/nodeselectors/gpu-nvidia.yaml
```

### Option 3: Edit Existing Deployment

```bash
kubectl edit daemonset kserve-kernelcachenode-agent -n kserve
# Add nodeSelector to spec.template.spec:
#   nodeSelector:
#     nvidia.com/gpu: "true"
```

## Common Scenarios

### NVIDIA GPU Cluster

```yaml
# my-overlay/kustomization.yaml
patches:
- target:
    kind: DaemonSet
    name: kserve-kernelcachenode-agent
  patch: |-
    apiVersion: apps/v1
    kind: DaemonSet
    metadata:
      name: kserve-kernelcachenode-agent
    spec:
      template:
        spec:
          nodeSelector:
            nvidia.com/gpu: "true"
```

Nodes are usually labeled automatically by the NVIDIA device plugin.

### AMD GPU Cluster

```yaml
patches:
- target:
    kind: DaemonSet
    name: kserve-kernelcachenode-agent
  patch: |-
    apiVersion: apps/v1
    kind: DaemonSet
    metadata:
      name: kserve-kernelcachenode-agent
    spec:
      template:
        spec:
          nodeSelector:
            amd.com/gpu: "true"
```

Nodes are usually labeled automatically by the AMD device plugin.

### Mixed GPU Types (NVIDIA or AMD)

Use nodeAffinity to match multiple labels:

```yaml
patches:
- target:
    kind: DaemonSet
    name: kserve-kernelcachenode-agent
  patch: |-
    apiVersion: apps/v1
    kind: DaemonSet
    metadata:
      name: kserve-kernelcachenode-agent
    spec:
      template:
        spec:
          affinity:
            nodeAffinity:
              requiredDuringSchedulingIgnoredDuringExecution:
                nodeSelectorTerms:
                - matchExpressions:
                  - key: nvidia.com/gpu
                    operator: Exists
                - matchExpressions:
                  - key: amd.com/gpu
                    operator: Exists
```

### Testing/Dev Clusters (KIND, minikube)

```bash
# Label worker nodes
kubectl label nodes -l '!node-role.kubernetes.io/control-plane' kserve/kernelcache=worker
```

```yaml
patches:
- target:
    kind: DaemonSet
    name: kserve-kernelcachenode-agent
  patch: |-
    apiVersion: apps/v1
    kind: DaemonSet
    metadata:
      name: kserve-kernelcachenode-agent
    spec:
      template:
        spec:
          nodeSelector:
            kserve/kernelcache: "worker"
```

### AWS GPU Instances

```yaml
patches:
- target:
    kind: DaemonSet
    name: kserve-kernelcachenode-agent
  patch: |-
    apiVersion: apps/v1
    kind: DaemonSet
    metadata:
      name: kserve-kernelcachenode-agent
    spec:
      template:
        spec:
          nodeSelector:
            node.kubernetes.io/instance-type: "g5.xlarge"  # or p4d.24xlarge, etc.
```

### Specific Availability Zone

```yaml
patches:
- target:
    kind: DaemonSet
    name: kserve-kernelcachenode-agent
  patch: |-
    apiVersion: apps/v1
    kind: DaemonSet
    metadata:
      name: kserve-kernelcachenode-agent
    spec:
      template:
        spec:
          nodeSelector:
            topology.kubernetes.io/zone: "us-west-2a"
```

## Tolerations for GPU Node Taints

### Why GPU Nodes Are Tainted

Production GPU clusters often taint GPU nodes to prevent non-GPU workloads from
consuming expensive GPU resources.
When a GPU node is added to a cluster, device plugins (NVIDIA GPU Operator, AMD
device plugin) or cloud providers (GKE, AKS, EKS) automatically apply taints such
as `nvidia.com/gpu=present:NoSchedule`.
This ensures that only pods explicitly requesting GPUs or explicitly tolerating
the taint can be scheduled on these expensive resources, preventing resource waste
from non-GPU workloads landing on GPU nodes.

Common GPU node taints include:
- NVIDIA: `nvidia.com/gpu=present:NoSchedule` (NVIDIA GPU Operator, GKE)
- AMD: `amd.com/gpu=present:NoSchedule` (AMD device plugin)
- Custom: `dedicated=gpu-workloads:NoSchedule` (organization-specific policies)

### Why KernelCache Agent Needs Explicit Tolerations

InferenceService and LLMInferenceService pods that request GPU resources
(e.g., `nvidia.com/gpu: 1`) automatically receive matching tolerations through
Kubernetes' ExtendedResourceToleration admission controller.
This means users do not need to manually add tolerations to their InferenceService
YAML when requesting GPUs — Kubernetes handles it automatically.

However, the KernelCache agent DaemonSet does not request GPU resources in its pod
spec because it is a management agent, not a GPU workload.
It monitors the node and manages kernel cache files but does not need GPU access
itself.
Because the agent does not request GPU resources, the ExtendedResourceToleration
admission controller does not add tolerations automatically.

To run the KernelCache agent on GPU nodes with taints, you must explicitly configure
tolerations that match your GPU node taints.
The agent needs to run on GPU nodes because that is where InferenceService pods mount
the cached kernels.
Use the examples below to add the appropriate tolerations for your cluster's GPU
configuration.

### Pre-made Toleration Examples

- **gpu-nvidia-toleration.yaml** - NodeSelector + toleration for NVIDIA GPUs
- **gpu-amd-toleration.yaml** - NodeSelector + toleration for AMD GPUs
- **custom-tolerations.yaml** - Multiple tolerations example

### NVIDIA GPU Cluster with Taint

```yaml
# my-overlay/kustomization.yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
- ../../config/kernelcachenodes

patches:
- target:
    kind: DaemonSet
    name: kserve-kernelcachenode-agent
  patch: |-
    apiVersion: apps/v1
    kind: DaemonSet
    metadata:
      name: kserve-kernelcachenode-agent
    spec:
      template:
        spec:
          nodeSelector:
            nvidia.com/gpu: "true"
          tolerations:
            - key: nvidia.com/gpu
              operator: Exists
              effect: NoSchedule
```

**Notes:**
- `operator: Exists` tolerates any value (recommended)
- NVIDIA GPU Operator automatically applies this taint
- GKE automatically taints GPU nodes with `nvidia.com/gpu=present:NoSchedule`

### AMD GPU Cluster with Taint

```yaml
patches:
- target:
    kind: DaemonSet
    name: kserve-kernelcachenode-agent
  patch: |-
    apiVersion: apps/v1
    kind: DaemonSet
    metadata:
      name: kserve-kernelcachenode-agent
    spec:
      template:
        spec:
          nodeSelector:
            amd.com/gpu: "true"
          tolerations:
            - key: amd.com/gpu
              operator: Exists
              effect: NoSchedule
```

### Mixed GPU Types with Tolerations

Use nodeAffinity for multiple GPU labels + tolerations for both taints:

```yaml
patches:
- target:
    kind: DaemonSet
    name: kserve-kernelcachenode-agent
  patch: |-
    apiVersion: apps/v1
    kind: DaemonSet
    metadata:
      name: kserve-kernelcachenode-agent
    spec:
      template:
        spec:
          affinity:
            nodeAffinity:
              requiredDuringSchedulingIgnoredDuringExecution:
                nodeSelectorTerms:
                - matchExpressions:
                  - key: nvidia.com/gpu
                    operator: Exists
                - matchExpressions:
                  - key: amd.com/gpu
                    operator: Exists
          tolerations:
            - key: nvidia.com/gpu
              operator: Exists
              effect: NoSchedule
            - key: amd.com/gpu
              operator: Exists
              effect: NoSchedule
```

### Custom Taints (Multiple Tolerations)

For clusters with multiple taints (GPU + custom):

```yaml
patches:
- target:
    kind: DaemonSet
    name: kserve-kernelcachenode-agent
  patch: |-
    apiVersion: apps/v1
    kind: DaemonSet
    metadata:
      name: kserve-kernelcachenode-agent
    spec:
      template:
        spec:
          nodeSelector:
            nvidia.com/gpu: "true"
          tolerations:
            # Standard GPU taint
            - key: nvidia.com/gpu
              operator: Exists
              effect: NoSchedule
            # Custom taint for dedicated GPU nodes
            - key: dedicated
              operator: Equal
              value: gpu-workloads
              effect: NoSchedule
```

### Cloud Provider Examples

**GKE (Google Kubernetes Engine):**
```yaml
# GKE auto-taints GPU nodes with nvidia.com/gpu=present:NoSchedule
tolerations:
  - key: nvidia.com/gpu
    operator: Exists
    effect: NoSchedule
```

**AKS (Azure Kubernetes Service):**
```yaml
# AKS uses sku=gpu taint on GPU node pools
nodeSelector:
  accelerator: nvidia
tolerations:
  - key: sku
    operator: Equal
    value: gpu
    effect: NoSchedule
```

**EKS (Amazon Elastic Kubernetes Service):**
```yaml
# EKS uses node labels, taints vary by setup
nodeSelector:
  node.kubernetes.io/instance-type: "g5.xlarge"
tolerations:
  - key: nvidia.com/gpu
    operator: Exists
    effect: NoSchedule
```

### Testing Toleration Configuration

```bash
# Check if GPU nodes have taints
kubectl get nodes -o json | jq '.items[] | select(.metadata.labels["nvidia.com/gpu"]=="true") | {name: .metadata.name, taints: .spec.taints}'

# Verify agent pods scheduled on GPU nodes
kubectl get pods -n kserve -l app=kserve-kernelcachenode-agent -o wide

# Check pod tolerations
kubectl get daemonset kserve-kernelcachenode-agent -n kserve -o jsonpath='{.spec.template.spec.tolerations}'
```

## Makefile Integration

For production deployments:
```bash
make deploy-kernelcache              # Deploy with default (all nodes)
kubectl apply -k my-overlay/         # Apply your custom overlay
```

For KIND testing:
```bash
make deploy-dev-kernelcache-kind     # Includes init container patch for KIND
# Then optionally apply nodeSelector via overlay or kubectl patch
```
