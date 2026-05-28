# PVC Initialization for Model Storage

This directory contains examples for initializing a Persistent Volume Claim (PVC) with model weights from HuggingFace, which is the recommended approach for deploying large language models in production.

## Overview

Instead of downloading model weights during pod startup (which can cause timeouts and resource waste), this approach pre-downloads model weights into a PVC using a Kubernetes Job. The LLMInferenceService pods then mount the PVC to access the pre-loaded model weights.

## Prerequisites

- Kubernetes cluster with a storage class that supports ReadWriteMany (RWX) access mode
- Service account with HuggingFace access (`hfsa`) - see [HuggingFace authentication documentation](https://kserve.github.io/website/docs/model-serving/storage/providers/hf?_highlight=hf_toke#private-hugging-face-models)
- Sufficient storage quota for your model (1Ti in the example)

## Files

- **`pvc.yaml`**: Creates a PersistentVolumeClaim for storing model weights
- **`job.yaml`**: Kubernetes Job that downloads model weights from HuggingFace to the PVC

## Usage

### 1. Create the PVC

First, create the PersistentVolumeClaim. You may need to adjust the `storageClassName` and `storage` size for your cluster:

```bash
kubectl apply -f pvc.yaml
```

Verify the PVC is created and bound:

```bash
kubectl get pvc llm-test-pvc
```

### 2. Run the Initialization Job

The job uses the KServe storage initializer to download model weights from HuggingFace:

```bash
kubectl apply -f job.yaml
```

Monitor the job progress:

```bash
# Check job status
kubectl get jobs deepseek-r1-0528-storage-init-job

# View logs
kubectl logs -f job/deepseek-r1-0528-storage-init-job
```

The job will download the model weights to `/mnt/models` in the PVC.

### 3. Use the PVC in Your LLMInferenceService

Once the job completes successfully, reference the PVC in your LLMInferenceService:

```yaml
spec:
  model:
    uri: pvc://llm-test-pvc
    name: deepseek-ai/DeepSeek-R1-0528
```

## Customization

### Different Model

To download a different model, modify the `args` in `job.yaml`:

```yaml
args:
  - hf://your-org/your-model-name
  - /mnt/models
```

And update the PVC name in both files if needed.

### Storage Class

The example uses `ibm-spectrum-scale-fileset` storage class. Update this to match your cluster's available storage classes:

```bash
kubectl get storageclass
```

Then modify `pvc.yaml`:

```yaml
spec:
  storageClassName: your-storage-class
```

### Storage Size

Adjust the storage request based on your model size. For reference:
- DeepSeek-R1-0528: ~600GB (1Ti provides buffer)
- Smaller models (7B parameters): ~20-30GB
- Medium models (70B parameters): ~150-200GB

## Performance Tuning

The job includes environment variables for optimized HuggingFace downloads:

- `HF_XET_NUM_CONCURRENT_RANGE_GETS=8`: Parallel downloads
- `HF_XET_HIGH_PERFORMANCE=True`: High-performance mode
- `HF_HUB_DISABLE_TELEMETRY=1`: Disable telemetry

Adjust these based on your network bandwidth and storage performance.

## Troubleshooting

### Job Fails with Authentication Error

Ensure your `hfsa` service account has proper HuggingFace credentials configured. See the [HuggingFace authentication guide](https://kserve.github.io/website/docs/model-serving/storage/providers/hf?_highlight=hf_toke#private-hugging-face-models).

### PVC Not Binding

Check if your storage class supports ReadWriteMany access mode:

```bash
kubectl describe storageclass <your-storage-class>
```

### Insufficient Storage

If the job fails with out-of-space errors, increase the PVC size in `pvc.yaml` and recreate it.

### Slow Downloads

- Increase `HF_XET_NUM_CONCURRENT_RANGE_GETS` for faster parallel downloads
- Ensure adequate network bandwidth between cluster and HuggingFace
- Check resource limits on the job pod

## Benefits Over Direct HuggingFace URIs

Using PVC initialization offers several advantages:

1. **Faster pod startup**: Models are pre-loaded, avoiding download during deployment
2. **Reliability**: Separates model download from inference pod lifecycle
3. **Resource efficiency**: Download once, use across multiple pods/replicas
4. **Cost savings**: Reduces HuggingFace bandwidth usage for repeated deployments
5. **Air-gapped support**: Enable deployments in restricted network environments