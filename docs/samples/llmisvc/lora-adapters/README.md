# LoRA Adapter Examples

This directory contains example configurations for deploying LLM inference services with LoRA (Low-Rank Adaptation) adapters.

## Overview

LoRA adapters allow you to serve a base model with task-specific or domain-specific adaptations without duplicating the full model weights. The LLMInferenceService controller automatically downloads adapters and configures the vLLM runtime to use them.

## What are LoRA Adapters?

**LoRA (Low-Rank Adaptation)** is a parameter-efficient fine-tuning technique that adapts large language models by training small adapter modules instead of the full model ([vLLM LoRA docs](https://docs.vllm.ai/en/stable/features/lora.html)). Benefits include:

- **Storage Efficiency**: Adapters are typically 1-100 MB vs. multi-GB base models
- **Multi-Tenancy**: Serve multiple adapted versions of a model from a single deployment
- **Fast Switching**: Load and unload adapters dynamically without restarting the service
- **Task Specialization**: Different adapters for translation, summarization, coding, etc.

## Prerequisites

- Kubernetes cluster with GPU nodes
- Base model accessible via HuggingFace, S3, or PVC
- LoRA adapter weights accessible via supported URI schemes
- vLLM-compatible runtime (default for LLMInferenceService)

## Supported URI Schemes

The controller supports three URI schemes for LoRA adapters:

| Scheme    | Description                          | Example                                      | Use Case                        |
|-----------|--------------------------------------|----------------------------------------------|---------------------------------|
| `hf://`   | HuggingFace Hub                      | `hf://my-org/my-lora-adapter`                | Public or authenticated HF repo |
| `s3://`   | S3-compatible object storage         | `s3://my-bucket/adapters/lora-v1`            | Private storage, large adapters |
| `pvc://`  | Kubernetes PersistentVolumeClaim     | `pvc://my-pvc/path/to/adapter`               | Pre-downloaded or shared PVC    |

**Note**: `oci://` is not supported for LoRA adapters. OCI models run as sidecar containers with shared process namespaces, but only one OCI sidecar per pod is currently supported. Workaround: package adapters in a PVC and use `pvc://`.

## Examples

### 1. Single HuggingFace LoRA Adapter ([llm-inference-service-lora-hf.yaml](llm-inference-service-lora-hf.yaml))

Deploy a base model with one LoRA adapter from HuggingFace Hub.

**Configuration:**
- Base Model: Qwen2.5-7B-Instruct
- Adapter: Single HuggingFace adapter
- Replicas: 2

**Use Case:**
- Public HuggingFace adapters
- Quick testing and development
- No additional storage setup required

**Deployment:**
```bash
kubectl apply -f llm-inference-service-lora-hf.yaml
```

**YAML snippet:**
```yaml
spec:
  model:
    uri: hf://Qwen/Qwen2.5-7B-Instruct
    name: Qwen/Qwen2.5-7B-Instruct
    lora:
      adapters:
        - name: sql-adapter
          uri: hf://my-org/qwen-sql-lora
```

### 2. Multiple LoRA Adapters ([llm-inference-service-lora-multi.yaml](llm-inference-service-lora-multi.yaml))

Deploy a base model with multiple LoRA adapters from different sources.

**Configuration:**
- Base Model: Qwen2.5-7B-Instruct
- Adapters:
  - HuggingFace adapter for SQL generation
  - S3 adapter for code translation
  - PVC adapter for domain-specific tasks
- Replicas: 2

**Use Case:**
- Multi-tenant deployments
- Multiple task-specific adapters
- Mixed storage backends

**Deployment:**
```bash
# Ensure S3 credentials are configured (if using s3://)
kubectl create secret generic s3-creds \
  --from-literal=AWS_ACCESS_KEY_ID=<key> \
  --from-literal=AWS_SECRET_ACCESS_KEY=<secret>

# Create PVC with adapter weights (if using pvc://)
kubectl apply -f adapter-pvc.yaml

# Deploy service
kubectl apply -f llm-inference-service-lora-multi.yaml
```

**YAML snippet:**
```yaml
spec:
  model:
    uri: hf://Qwen/Qwen2.5-7B-Instruct
    name: Qwen/Qwen2.5-7B-Instruct
    lora:
      adapters:
        - name: sql-adapter
          uri: hf://my-org/qwen-sql-lora
        - name: code-adapter
          uri: s3://my-bucket/adapters/code-lora
        - name: domain-adapter
          uri: pvc://adapter-pvc/domain-lora
```

### 3. S3 LoRA Adapter with Custom Endpoint ([llm-inference-service-lora-s3.yaml](llm-inference-service-lora-s3.yaml))

Deploy with a LoRA adapter from S3-compatible storage (MinIO, Ceph, etc.).

**Configuration:**
- Base Model: Qwen2.5-7B-Instruct
- Adapter: S3-compatible storage
- Custom S3 endpoint configured
- Replicas: 2

**Use Case:**
- Private object storage
- On-premises deployments
- Custom S3-compatible backends (MinIO, Ceph)

**Deployment:**
```bash
# Create S3 configuration
kubectl create secret generic s3-config \
  --from-literal=AWS_ACCESS_KEY_ID=<key> \
  --from-literal=AWS_SECRET_ACCESS_KEY=<secret> \
  --from-literal=S3_ENDPOINT=https://minio.example.com \
  --from-literal=S3_USE_HTTPS=1

kubectl apply -f llm-inference-service-lora-s3.yaml
```

### 4. PVC-Based LoRA Adapter ([llm-inference-service-lora-pvc.yaml](llm-inference-service-lora-pvc.yaml))

Deploy with a LoRA adapter stored in a PersistentVolumeClaim.

**Configuration:**
- Base Model: Qwen2.5-7B-Instruct
- Adapter: PVC storage
- Replicas: 2

**Use Case:**
- Pre-downloaded adapters
- Shared storage across services
- Air-gapped environments
- No storage-initializer overhead (adapter already on disk)

**Deployment:**
```bash
# Create PVC and populate with adapter weights
kubectl apply -f adapter-pvc.yaml

# Copy adapter weights to PVC (example using a job)
kubectl run -it --rm copy-adapter --image=busybox --restart=Never -- sh
# Inside pod: download and extract adapter to /mnt/adapter

# Deploy service
kubectl apply -f llm-inference-service-lora-pvc.yaml
```

**YAML snippet:**
```yaml
spec:
  model:
    uri: hf://Qwen/Qwen2.5-7B-Instruct
    name: Qwen/Qwen2.5-7B-Instruct
    lora:
      adapters:
        - name: my-adapter
          uri: pvc://adapter-pvc/my-lora-adapter
```

## How It Works

### Automatic LoRA Integration

When you specify `spec.model.lora.adapters`, the controller automatically:

1. **Downloads Adapters** (for `hf://` and `s3://`):
   - Runs storage-initializer as an init container
   - Downloads all adapters in parallel
   - Mounts adapters to `/mnt/lora/<adapter-name>`

2. **Mounts PVC Adapters** (for `pvc://`):
   - Creates volume mounts for each PVC
   - Mounts to `/mnt/lora/<adapter-name>`
   - Read-only to prevent accidental modification

3. **Configures vLLM Runtime**:
   - Appends `--enable-lora` to vLLM CLI arguments
   - Sets `--max-lora-rank` from `spec.model.lora.maxRank` (default: 64)
   - Sets `--max-loras` from `spec.model.lora.maxAdapters` (default: number of configured adapters)
   - Sets `--max-cpu-loras` from `spec.model.lora.maxCpuAdapters` (default: number of configured adapters)
   - Passes `--lora-modules adapter-name=/mnt/lora/adapter-name` for each adapter

### Adapter Path Sanitization

Adapter names are sanitized to create valid filesystem paths:
- Invalid characters (`/`, `\`, `:`, etc.) are replaced with `-`
- Only alphanumeric, dash, underscore, and dot are allowed
- Example: `my/adapter:v1` → `my-adapter-v1`

### Storage Initializer

For `hf://` and `s3://` adapters:
- **Enabled by default** (unless `spec.storageInitializer.enabled: false`)
- Downloads all adapters in a single init container run
- Uses parallel downloads for efficiency
- Shares the same emptyDir volume with the main container

For `pvc://` adapters:
- **No storage-initializer required**
- PVC is mounted directly to the pod
- Faster startup (no download phase)

## Using LoRA Adapters at Inference Time

### OpenAI-Compatible API

Use the `model` parameter in requests to select which adapter to use:

```bash
# Request using the sql-adapter
curl -k https://<route-url>/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "sql-adapter",
    "messages": [
      {"role": "user", "content": "Generate SQL for: show all users"}
    ]
  }'

# Request using the base model (no adapter)
curl -k https://<route-url>/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "Qwen/Qwen2.5-7B-Instruct",
    "messages": [
      {"role": "user", "content": "What is Kubernetes?"}
    ]
  }'
```

### Adapter Selection

- **With adapter**: Set `"model": "<adapter-name>"` (matches `spec.model.lora.adapters[].name`)
- **Without adapter**: Set `"model": "<base-model-name>"` (matches `spec.model.name`)
- vLLM automatically loads the appropriate adapter weights for each request

## Configuration Details

### LoRA Rank Limit

The controller sets `--max-lora-rank=64` by default. This must be ≥ the rank used during adapter training.

If your adapters use a higher rank, set `spec.model.lora.maxRank`:

```yaml
spec:
  model:
    lora:
      maxRank: 128
      adapters:
        - name: my-adapter
          uri: hf://my-org/my-high-rank-adapter
```

### Multiple Adapters

All adapters are loaded at service startup. vLLM can switch between adapters dynamically per request without reloading.

Use `spec.model.lora.maxAdapters` and `spec.model.lora.maxCpuAdapters` to control how many adapters are kept in GPU and CPU memory respectively:

```yaml
spec:
  model:
    lora:
      maxAdapters: 2      # max adapters in GPU memory simultaneously (default: number of adapters)
      maxCpuAdapters: 4   # max adapters cached in CPU memory (default: number of adapters)
      adapters:
        - name: adapter-1
          uri: hf://my-org/adapter-1
        - name: adapter-2
          uri: hf://my-org/adapter-2
```

**Resource impact:**
- Each adapter consumes GPU memory proportional to its rank
- Higher `maxAdapters` increases GPU memory overhead
- Use `maxCpuAdapters` to cache adapters in CPU memory when not actively serving
- Monitor GPU memory usage when serving many adapters

### Adapter Name Constraints

- Adapter names must be unique within a service
- Adapter names must differ from the base model name
- Names are case-sensitive

## Monitoring and Verification

### Check Pod Status

```bash
# List pods
kubectl get pods -l app.kubernetes.io/component=llminferenceservice-workload

# Check init container logs (for hf:// or s3:// adapters)
kubectl logs <pod-name> -c storage-initializer

# Check main container logs
kubectl logs <pod-name> -c main
```

### Verify LoRA Loading

Look for these messages in the main container logs:

```
INFO: Loaded new LoRA adapter: name 'sql-adapter', path '/mnt/lora/sql-adapter'
INFO: Loaded new LoRA adapter: name 'code-adapter', path '/mnt/lora/code-adapter'
```

### Check vLLM Configuration

```bash
kubectl logs <pod-name> -c main | grep -E "enable.lora|max.lora|lora.modules"
```

Expected output:
```
'enable_lora': True
'max_lora_rank': 64
'max_loras': 2
'max_cpu_loras': 2
'lora_modules': [LoRAModulePath(name='sql-adapter', path='/mnt/lora/sql-adapter'), ...]
```

## Limitations

### Unsupported Features

- **OCI adapters** (`oci://`): Not supported due to single-modelcar-per-pod limitation
  - Workaround: Download OCI image contents to PVC, use `pvc://`

### Storage Initializer Dependency

- `hf://` and `s3://` adapters require `spec.storageInitializer.enabled: true` (default)
- Setting `storageInitializer.enabled: false` will cause validation errors for these schemes
- `pvc://` adapters work without storage-initializer

### Runtime Constraints

- All adapters must be compatible with the base model architecture
- Adapters must use LoRA format compatible with vLLM
- Maximum rank is limited by `spec.model.lora.maxRank` (default 64); increase it if adapters were trained with a higher rank

## Troubleshooting

### Adapters Not Loading

**Symptom**: vLLM starts but adapters are not listed in logs

**Possible causes:**
1. **Incorrect URI format**: Ensure URI has proper scheme (`hf://`, `s3://`, `pvc://`)
2. **Storage-initializer disabled**: For `hf://` or `s3://`, ensure `storageInitializer.enabled` is not false
3. **PVC not mounted**: For `pvc://`, verify PVC exists and is bound

**Debugging:**
```bash
# Check init container status
kubectl describe pod <pod-name>

# View storage-initializer logs
kubectl logs <pod-name> -c storage-initializer

# Check volume mounts
kubectl get pod <pod-name> -o jsonpath='{.spec.containers[0].volumeMounts}' | jq
```

### Download Failures (HuggingFace)

**Symptom**: Storage-initializer fails with authentication error

**Solution:**
```bash
# Create HuggingFace token secret
kubectl create secret generic hf-token \
  --from-literal=HF_TOKEN=<your-token>

# Reference in LLMInferenceService
spec:
  template:
    containers:
      - name: storage-initializer
        env:
          - name: HF_TOKEN
            valueFrom:
              secretKeyRef:
                name: hf-token
                key: HF_TOKEN
```

### Download Failures (S3)

**Symptom**: Storage-initializer fails to download from S3

**Solution:**
```bash
# Verify S3 credentials
kubectl create secret generic s3-creds \
  --from-literal=AWS_ACCESS_KEY_ID=<key> \
  --from-literal=AWS_SECRET_ACCESS_KEY=<secret>

# For custom S3 endpoints (MinIO, etc.)
kubectl create secret generic s3-config \
  --from-literal=AWS_ACCESS_KEY_ID=<key> \
  --from-literal=AWS_SECRET_ACCESS_KEY=<secret> \
  --from-literal=S3_ENDPOINT=https://minio.example.com \
  --from-literal=S3_USE_HTTPS=1
```

### PVC Mount Failures

**Symptom**: Pod fails to start with volume mount error

**Possible causes:**
1. PVC does not exist
2. PVC is not bound to a PV
3. PVC access mode incompatible (must support ReadOnlyMany or ReadWriteMany for multiple replicas)

**Debugging:**
```bash
# Check PVC status
kubectl get pvc <pvc-name>

# Check PV binding
kubectl describe pvc <pvc-name>
```

### Adapter Name Conflicts

**Symptom**: Validation error: "LoRA adapter name X must differ from base model name Y"

**Solution**: Ensure each adapter has a unique name that differs from `spec.model.name`.

### Out of Memory Errors

**Symptom**: vLLM crashes with CUDA OOM error when loading adapters

**Possible causes:**
- Too many adapters loaded simultaneously
- Adapters with high rank
- Insufficient GPU memory

**Solutions:**
1. Reduce number of adapters
2. Use lower-rank adapters (re-train if necessary)
3. Increase GPU memory allocation
4. Set `spec.model.lora.maxAdapters` to a smaller value if only a subset of adapters need to be in GPU memory at once

## Performance Considerations

### Adapter Switching Overhead

- Switching between adapters has minimal latency impact (~1-5ms)
- vLLM keeps all loaded adapters in GPU memory
- No reloading required when switching between requests

### Memory Usage

Approximate GPU memory per adapter:
```
adapter_memory ≈ rank × num_layers × hidden_dim × 2 × sizeof(fp16)
```

For a 7B model with rank=64:
- Typical adapter: 50-100 MB GPU memory
- 10 adapters: 500MB-1GB additional GPU memory

### Download Performance

For `hf://` and `s3://`:
- Download time depends on adapter size and network bandwidth
- Multiple adapters are downloaded in parallel
- Use `pvc://` to avoid download overhead for frequently used adapters

## Advanced Configuration

### Custom vLLM Arguments

Prefer using the spec fields (`maxRank`, `maxAdapters`, `maxCpuAdapters`) over `VLLM_ADDITIONAL_ARGS` for LoRA configuration. Use `VLLM_ADDITIONAL_ARGS` only when you need to pass other vLLM flags alongside LoRA settings:

```yaml
spec:
  model:
    lora:
      maxRank: 128
      maxAdapters: 2
      maxCpuAdapters: 4
      adapters:
        - name: my-adapter
          uri: hf://my-org/my-adapter
```

**Note**: If you manually specify `--lora-modules` in `VLLM_ADDITIONAL_ARGS` or container args, the controller will skip automatic LoRA injection.

### Pre-download to PVC

For production deployments, consider pre-downloading adapters to PVC:

```bash
# Create job to download adapters
apiVersion: batch/v1
kind: Job
metadata:
  name: download-adapters
spec:
  template:
    spec:
      containers:
      - name: downloader
        image: quay.io/kserve/storage-initializer:latest
        command: ["/storage-initializer"]
        args:
          - "hf://my-org/adapter1"
          - "/mnt/adapters/adapter1"
          - "hf://my-org/adapter2"
          - "/mnt/adapters/adapter2"
        volumeMounts:
        - name: adapter-storage
          mountPath: /mnt/adapters
      volumes:
      - name: adapter-storage
        persistentVolumeClaim:
          claimName: adapter-pvc
      restartPolicy: Never
```

Then reference in LLMInferenceService:
```yaml
spec:
  model:
    lora:
      adapters:
        - name: adapter1
          uri: pvc://adapter-pvc/adapter1
        - name: adapter2
          uri: pvc://adapter-pvc/adapter2
```

Benefits:
- Faster pod startup (no download phase)
- Consistent adapter versions across replicas
- Works in air-gapped environments
