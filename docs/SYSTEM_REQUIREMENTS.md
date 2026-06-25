# KServe System Requirements

This document describes the minimum hardware and system requirements for running KServe. Requirements vary by deployment mode and workload type.

## Kubernetes Version Compatibility

| Kubernetes Version | KServe Support |
|-------------------|----------------|
| 1.28+             | Supported      |
| 1.32+             | Recommended for latest features |

## Control Plane Requirements

KServe control plane components have the following resource requirements (from `charts/kserve-resources/values.yaml`):

| Component           | CPU Request | CPU Limit | Memory Request | Memory Limit |
|---------------------|-------------|-----------|----------------|--------------|
| Controller Manager  | 100m        | 100m      | 300Mi          | 300Mi        |
| RBAC Proxy          | 100m        | 100m      | 300Mi          | 300Mi        |
| Storage Initializer | 100m        | 1         | 100Mi          | 1Gi          |
| Modelcar (sidecar)  | 10m         | —         | 15Mi           | —            |

## Cluster Requirements by Deployment Mode

### Standard (RawDeployment)

Lightweight installation without Knative. Does not support canary deployment or request-based autoscaling with scale-to-zero.

| Resource   | Minimum per Node |
|------------|------------------|
| CPU        | 2 cores          |
| Memory     | 4 GB             |
| Storage    | 20 GB            |
| Nodes      | 1+               |

### Serverless (Knative)

Default installation with Knative for serverless deployment and scale-to-zero.

| Resource   | Minimum per Node |
|------------|------------------|
| CPU        | 4 cores          |
| Memory     | 8 GB             |
| Storage    | 50 GB            |
| Nodes      | 2+               |

### ModelMesh

For high-scale, high-density, and frequently-changing model serving.

| Resource   | Minimum per Node |
|------------|------------------|
| CPU        | 4 cores          |
| Memory     | 16 GB            |
| Storage    | 100 GB           |
| Nodes      | 3+               |

## Local Development (Minikube)

For running KServe locally on a single-node Minikube cluster:

```bash
minikube start --memory=16384 --cpus=4 --disk-size=50g --kubernetes-version=v1.26.1
```

| Resource   | Minimum      |
|------------|--------------|
| CPU        | 4 cores      |
| Memory     | 16 GB        |
| Disk       | 50 GB        |

For more demanding workloads (e.g., multiple InferenceServices), consider:

```bash
minikube start --memory=16384 --cpus=10 --disk-size=50g
```

## InferenceService Defaults

Default resource requests/limits for InferenceService pods (configurable):

| Resource | Request | Limit |
|----------|---------|-------|
| CPU      | 1       | 1     |
| Memory   | 2Gi     | 2Gi   |

Adjust these based on your model size and expected load.

## GPU Requirements (LLM Workloads)

For GPU-accelerated LLM inference:

| Model Size | GPU VRAM | CPU  | System RAM |
|------------|----------|------|------------|
| 7B params  | 16 GB+   | 4+   | 32 GB+     |
| 13B params | 24 GB+   | 4+   | 64 GB+     |
| 70B params | 80 GB+   | 8+   | 128 GB+    |

**Prerequisites:**
- NVIDIA GPU Operator or device plugin installed
- CUDA compute capability 7.0 or higher
- Appropriate drivers and container runtime (nvidia-container-toolkit)

## Network and Storage

- **Network:** Standard Kubernetes networking. For production, ensure sufficient bandwidth for model downloads and inference traffic.
- **Storage:** Persistent volume support required for model storage. Storage class must match your deployment (e.g., ReadWriteMany for multi-replica model sharing).

## Production Recommendations

1. **High availability:** Use 3+ nodes for control plane and workload redundancy.
2. **Resource quotas:** Set namespace-level quotas to prevent resource exhaustion.
3. **Monitoring:** Deploy Prometheus and Grafana for observability.
4. **Storage:** Use fast storage (SSD/NVMe) for model loading performance.

## Quick Reference

| Scenario              | CPU  | Memory | Storage | Nodes |
|-----------------------|------|--------|---------|-------|
| Minikube (local)      | 4    | 16 GB  | 50 GB   | 1     |
| Standard deployment   | 2/node | 4 GB/node | 20 GB | 1+    |
| Serverless (Knative)  | 4/node | 8 GB/node | 50 GB | 2+    |
| ModelMesh             | 4/node | 16 GB/node | 100 GB | 3+    |
| LLM 7B (GPU)         | 4+   | 32 GB+ | 50 GB+  | 1+    |

## See Also

- [Quick Installation Guide](https://kserve.github.io/website/docs/getting-started/quickstart-guide)
- [Admin Guide - Installation](https://kserve.github.io/website/docs/admin-guide/overview)
- [Helm Chart Values](https://github.com/kserve/kserve/tree/main/charts/kserve-resources)
