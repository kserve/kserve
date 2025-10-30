# DeepSeek-R1 Multi-Node Deployment Examples

This directory contains example configurations for deploying the DeepSeek-R1-0528 model using data parallelism (DP) and
expert parallelism (EP) across multiple nodes with GPU acceleration.

## Prerequisites

- Kubernetes cluster with GPU nodes
- RoCE (RDMA over Converged Ethernet) network configuration
- Model weights stored in PVC (recommended) or accessible via HuggingFace
- Service account with HuggingFace access (`hfsa`) - see
  the [HuggingFace authentication guide](https://kserve.github.io/website/docs/model-serving/storage/providers/hf?_highlight=hf_toke#private-hugging-face-models)
  or the [PVC initialization guide](./../../../storage/pvc-init) for the recommended approach
- Proper RDMA and GPU resource quotas

## Network Configuration

All examples require RoCE networking. See `network-roce.yaml` for cluster-specific network attachment configuration.

## Examples

### 1. Basic DP+EP Configuration ([`llm-inference-service-dp-ep-deepseek-r1-gpu-deepep-ht.yaml`](llm-inference-service-dp-ep-deepseek-r1-gpu-deepep-ht.yaml))

Basic multi-node deployment with high-throughput all-to-all backend.

**Configuration:**

- Data parallelism: 32
- Data-local parallelism: 8
- Total replicas: 4 (calculated as `data / dataLocal = 32 / 8`)
- Expert parallelism: enabled
- All2All backend: `deepep_high_throughput`
- GPUs per node: 8
- Max sequence length: 8192

**Key features:**

- NVSHMEM with IBGDA transport for efficient inter-node communication
- GPU memory utilization: 0.95
- RoCE networking with GID index 3

### 2. Prefill-Decode Separation with High-Throughput Backend ([`llm-inference-service-dp-ep-pd-deepseek-r1-gpu-p-deepep-ht-d-deepep-ht.yaml`](llm-inference-service-dp-ep-pd-deepseek-r1-gpu-p-deepep-ht-d-deepep-ht.yaml))

Optimized configuration that separates prefill and decode workloads for better resource utilization.

**Configuration:**

- Main/Decode pool:
    - Data parallelism: 16
    - Data-local parallelism: 8
    - Total replicas: 2 (calculated as `data / dataLocal = 16 / 8`)
    - All2All backend: `deepep_high_throughput`
- Prefill pool:
    - Data parallelism: 16
    - Data-local parallelism: 8
    - Total replicas: 2 (calculated as `data / dataLocal = 16 / 8`)
    - All2All backend: `deepep_high_throughput`
- KV cache transfer enabled via NixlConnector
- Prefix cache scoring with weight 2.0
- Load-aware scheduling

**Key features:**

- Separate prefill/decode pools for optimized performance
- KV cache transfer between pools
- Advanced scheduler configuration with prefix cache routing
- GPU memory utilization: 0.99 (main/decode), 0.97 (prefill)
- Max sequence length: 4096

### 3. Prefill-Decode with Mixed Backend ([`llm-inference-service-dp-ep-pd-deepseek-r1-gpu-p-deepep-ht-d-pplx.yaml`](llm-inference-service-dp-ep-pd-deepseek-r1-gpu-p-deepep-ht-d-pplx.yaml))

Hybrid configuration using different all-to-all backends for prefill and decode phases.

**Configuration:**

- Main/Decode pool:
    - Data parallelism: 16
    - Data-local parallelism: 8
    - Total replicas: 2 (calculated as `data / dataLocal = 16 / 8`)
    - All2All backend: `pplx`
- Prefill pool:
    - Data parallelism: 16
    - Data-local parallelism: 8
    - Total replicas: 2 (calculated as `data / dataLocal = 16 / 8`)
    - All2All backend: `deepep_high_throughput`
- KV cache transfer enabled via NixlConnector

**Key features:**

- Optimized decode performance with `pplx` backend
- High-throughput prefill processing
- Threshold-based prefill/decode separation (threshold: 0)
- GPU memory utilization: 0.99 (main/decode), 0.97 (prefill)
- Max sequence length: 4096

## All2All Backend Options

The examples demonstrate different all-to-all communication backends:

- **`deepep_high_throughput`**: Optimized for maximum throughput, suitable for batch processing
- **`pplx`**: Optimized for low-latency decode operations

## Scheduler Configuration

Examples with prefill-decode separation include advanced scheduler configurations:

- **pd-profile-handler**: Determines whether to use prefill or decode pool
- **prefix-cache-scorer**: Prioritizes endpoints with matching prefix cache (weight: 2.0)
- **load-aware-scorer**: Balances load across endpoints (weight: 1.0)
- **max-score-picker**: Selects endpoint with highest combined score

## Resource Requirements

Each worker pod requires:

- 8 NVIDIA GPUs
- 128 CPU cores (limit), 64 cores (request)
- 512Gi memory (limit), 256Gi memory (request)
- 800Gi ephemeral storage
- 1 RDMA/RoCE GDR resource

## Deployment

1. Ensure your cluster has the required network configuration:
   ```bash
   kubectl apply -f network-roce.yaml
   ```

2. Deploy the chosen configuration:
   ```bash
   kubectl apply -f <example-file>.yaml
   ```

3. Monitor deployment:
   ```bash
   kubectl get llminferenceservice -owide
   ```

## Important Notes

- Initial deployment can take up to 80 minutes (see `initialDelaySeconds: 4800`)
- Security contexts enforce non-root execution with capability restrictions
- NCCL and NVSHMEM environment variables are tuned for your environment, stability and performance
