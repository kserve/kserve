# Minimum Hardware and System Requirements

This document describes the minimum hardware and system requirements for deploying KServe.

## Kubernetes Version Requirements

| KServe Version | Kubernetes Version |
|----------------|-------------------|
| v0.14+         | 1.28+             |
| v0.12 - v0.13  | 1.27+             |
| v0.10 - v0.11  | 1.25+             |

## Control Plane Requirements

The KServe control plane consists of the controller and webhook components. Below are the default resource allocations:

### KServe Controller

| Component               | CPU Request | CPU Limit | Memory Request | Memory Limit |
|-------------------------|-------------|-----------|----------------|--------------|
| Controller Manager      | 100m        | 100m      | 300Mi          | 300Mi        |
| RBAC Proxy              | 100m        | 100m      | 300Mi          | 300Mi        |

### Storage Initializer

| Component               | CPU Request | CPU Limit | Memory Request | Memory Limit |
|-------------------------|-------------|-----------|----------------|--------------|
| Storage Initializer     | 100m        | 1         | 100Mi          | 1Gi          |

### Model Sidecar (Modelcar)

| Component               | CPU         | Memory    |
|-------------------------|-------------|-----------|
| Model Sidecar           | 10m         | 15Mi      |

## Cluster Requirements by Deployment Mode

### Standard (RawDeployment) Mode

This is the lightweight deployment option without serverless capabilities.

**Minimum cluster resources:**
- **Nodes**: 1+ worker nodes
- **CPU**: 2 cores per node (minimum)
- **Memory**: 4 GB per node (minimum)
- **Storage**: 20 GB available disk space

**Required components:**
- Kubernetes 1.28+
- cert-manager (for webhook certificates)

### Serverless (Knative) Mode

This mode provides autoscaling with scale-to-zero capabilities.

**Minimum cluster resources:**
- **Nodes**: 2+ worker nodes (recommended for high availability)
- **CPU**: 4 cores per node (minimum)
- **Memory**: 8 GB per node (minimum)
- **Storage**: 50 GB available disk space

**Required components:**
- Kubernetes 1.28+
- Knative Serving 1.13+
- Istio 1.20+ or Kourier (networking layer)
- cert-manager (for webhook certificates)

### ModelMesh Mode

For high-scale, high-density model serving use cases.

**Minimum cluster resources:**
- **Nodes**: 3+ worker nodes (recommended)
- **CPU**: 4 cores per node (minimum)
- **Memory**: 16 GB per node (minimum)
- **Storage**: 100 GB available disk space

**Required components:**
- Kubernetes 1.28+
- etcd (for model metadata)

## InferenceService Resource Requirements

Default resource allocations for model serving pods:

| Component               | CPU Request | CPU Limit | Memory Request | Memory Limit |
|-------------------------|-------------|-----------|----------------|--------------|
| InferenceService        | 1           | 1         | 2Gi            | 2Gi          |

These defaults can be overridden in the InferenceService specification.

## GPU Requirements

For GPU-accelerated inference:

- **NVIDIA GPU Operator** or **NVIDIA device plugin** installed
- **CUDA-compatible GPU** (compute capability 7.0+ recommended for LLM workloads)

### LLM Inference (LLMInferenceService)

For large language model serving, the following are typical requirements:

| Model Size | GPU Memory | CPU Cores | System Memory |
|------------|------------|-----------|---------------|
| 7B params  | 16 GB+     | 4+        | 32 GB+        |
| 13B params | 24 GB+     | 4+        | 64 GB+        |
| 70B params | 80 GB+     | 8+        | 128 GB+       |

Example resource configuration for LLM serving:
- GPU: 1 NVIDIA GPU per replica
- CPU: 4 cores (limit), 2 cores (request)
- Memory: 32Gi (limit), 16Gi (request)

## Multi-Model Serving Resource Overhead

When using Multi-Model Serving, additional resources are needed for the model agent sidecar:

| Component               | CPU         | Memory    |
|-------------------------|-------------|-----------|
| Agent Sidecar           | ~0.5 cores  | ~0.5 GB   |

This overhead is per InferenceService replica.

## Network Requirements

- **Ingress**: LoadBalancer or NodePort service for external access
- **DNS**: Cluster DNS (CoreDNS) for service discovery
- **Network bandwidth**: Depends on model size and inference throughput requirements

### Istio Requirements (if using Istio networking)

| Component               | CPU Request | Memory Request |
|-------------------------|-------------|----------------|
| Istio Ingress Gateway   | 100m        | 128Mi          |
| Istiod                  | 500m        | 2Gi            |

## Storage Requirements

- **Model storage**: Depends on model size and number of models
- **Supported storage backends**:
  - Amazon S3
  - Google Cloud Storage
  - Azure Blob Storage
  - HDFS
  - PersistentVolumeClaim (PVC)
  - HTTP/HTTPS URLs

## Production Recommendations

For production deployments, consider the following recommendations:

1. **High Availability**: Deploy 2+ replicas of KServe controller
2. **Resource Headroom**: Provision 20-30% extra resources for burst capacity
3. **Monitoring**: Deploy Prometheus and Grafana for metrics collection
4. **Node Pools**: Use dedicated node pools for inference workloads
5. **Autoscaling**: Configure cluster autoscaler for dynamic workload handling

## Quick Reference

| Deployment Mode | Min Nodes | Min CPU/Node | Min Memory/Node | Min Storage |
|-----------------|-----------|--------------|-----------------|-------------|
| Standard        | 1         | 2 cores      | 4 GB            | 20 GB       |
| Serverless      | 2         | 4 cores      | 8 GB            | 50 GB       |
| ModelMesh       | 3         | 4 cores      | 16 GB           | 100 GB      |

## Additional Resources

- [KServe Installation Guide](https://kserve.github.io/website/docs/admin-guide/overview)
- [Kubernetes Resource Management](https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/)
- [Helm Chart Configuration](../charts/kserve-resources/README.md)
