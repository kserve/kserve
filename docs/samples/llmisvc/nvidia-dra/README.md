# NVIDIA Dynamic Resource Allocation (DRA) Examples

Sample configurations for deploying `LLMInferenceService` workloads using Kubernetes **Dynamic Resource Allocation (DRA)** with NVIDIA GPUs, as an alternative to the legacy device plugin (`nvidia.com/gpu`).

These mirror the patterns in [`../single-node-gpu/`](../single-node-gpu/) but request GPUs via `ResourceClaimTemplate` and `resources.claims` instead of extended resources.

## Why DRA?

| Capability | Device plugin (`nvidia.com/gpu`) | DRA |
| :--- | :--- | :--- |
| Allocation model | Opaque extended resource | First-class `ResourceClaim` API objects |
| Per-container binding | Pod-level limit applies to all containers | GPU mapped only to containers that declare `resources.claims` |
| Device selection | Any GPU on the node | CEL selectors on model, VRAM, MIG profile, etc. |
| Replica scaling | One limit per pod | `ResourceClaimTemplate` creates one claim per pod |

KServe propagates `spec.template` (a `PodSpec`) directly to the underlying `Deployment`. Because `PodSpec.resourceClaims` and `ResourceRequirements.claims` are native Kubernetes fields, **no KServe controller changes are required**.

## Prerequisites

- Kubernetes v1.30+ with DRA enabled
- [NVIDIA GPU Operator](https://docs.nvidia.com/datacenter/cloud-native/gpu-operator/latest/dra-intro-install.html) with DRA driver installed
- `gpu.nvidia.com` DeviceClass present:

  ```bash
  kubectl get deviceclass
  ```

- Model weights accessible via HuggingFace or PVC
- For the prefill-decode example: RoCE network and RDMA support (same as [`../single-node-gpu/`](../single-node-gpu/))

## Files

| File | Description |
| :--- | :--- |
| [`nvidia-gpu-claim-template.yaml`](nvidia-gpu-claim-template.yaml) | Shared template — 1 GPU with >= 24 GiB VRAM per pod |
| [`llm-inference-service-qwen2-7b-nvidia-dra.yaml`](llm-inference-service-qwen2-7b-nvidia-dra.yaml) | Basic 3-replica deployment with scheduler |
| [`llm-inference-service-pd-qwen2-7b-nvidia-dra.yaml`](llm-inference-service-pd-qwen2-7b-nvidia-dra.yaml) | Prefill-decode separation with RDMA KV transfer |

## Deployment

### 1. Basic (3 replicas)

```bash
kubectl apply -f nvidia-gpu-claim-template.yaml
kubectl apply -f llm-inference-service-qwen2-7b-nvidia-dra.yaml
```

### 2. Prefill-decode with RDMA

```bash
kubectl apply -f nvidia-gpu-claim-template.yaml
kubectl apply -f llm-inference-service-pd-qwen2-7b-nvidia-dra.yaml
```

## How It Works

1. `ResourceClaimTemplate` defines the GPU requirements (device class, count, CEL filters).
2. `LLMInferenceService` references the template under `template.resourceClaims`.
3. KServe copies the `PodSpec` to the generated `Deployment`.
4. Kubernetes creates one `ResourceClaim` per pod replica from the template.
5. The GPU is bound only to the `main` container via `resources.claims` — sidecars and init containers do not receive a GPU.

```
User applies LLMInferenceService (replicas: 3)
        │
        ▼
KServe creates Deployment
        │
        ▼
Kubernetes creates 3 ResourceClaims (one per pod)
        │
        ▼
Scheduler allocates GPUs from ResourceSlices
        │
        ▼
GPU injected into main container only
```

## DRA vs Device Plugin

**Device plugin** ([`../single-node-gpu/llm-inference-service-qwen2-7b-gpu.yaml`](../single-node-gpu/llm-inference-service-qwen2-7b-gpu.yaml)):

```yaml
template:
  containers:
  - name: main
    resources:
      limits:
        nvidia.com/gpu: "1"
      requests:
        nvidia.com/gpu: "1"
```

**DRA** (this directory):

```yaml
template:
  resourceClaims:
  - name: gpu
    resourceClaimTemplateName: nvidia-gpu-template
  containers:
  - name: main
    resources:
      claims:
      - name: gpu
```

> **Important:** Always use `resourceClaimTemplateName`, not `resourceClaimName`. Static claim names cause all replicas to compete for a single GPU and leave extra pods stuck in `Pending`.

## Verification

```bash
# Service status
kubectl get llminferenceservice qwen2-7b-instruct-nvidia-dra -owide

# Per-replica ResourceClaims
kubectl get resourceclaims

# Claim allocation details
kubectl describe resourceclaim <claim-name>

# Pods running
kubectl get pods -l app.kubernetes.io/component=llminferenceservice-workload
```

## Troubleshooting

### Pods stuck in `Pending`

- Confirm `gpu.nvidia.com` DeviceClass exists: `kubectl get deviceclass`
- Check claim events: `kubectl describe resourceclaim <name>`
- Verify CEL selectors match your hardware: `kubectl get resourceslice -o yaml`
- Ensure GPU nodes are not tainted without matching tolerations

### ResourceClaim allocation failed

- DRA driver logs: `kubectl logs -l app.kubernetes.io/name=nvidia-dra-driver-gpu -n <driver-namespace>`
- Confirm the legacy NVIDIA device plugin is disabled when running DRA-only mode

### Migrating from device plugin samples

The DRA manifests in this directory are direct conversions of the [`single-node-gpu`](../single-node-gpu/) examples. To migrate an existing deployment:

1. Apply `nvidia-gpu-claim-template.yaml`.
2. Remove `nvidia.com/gpu` from `resources.limits` and `resources.requests`.
3. Add `resourceClaims` at the pod spec level (`template` / `prefill.template`).
4. Add `resources.claims` on the `main` container.
