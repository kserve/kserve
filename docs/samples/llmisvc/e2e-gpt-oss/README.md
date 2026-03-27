# End-to-end guide: Run GPT-OSS-20B with KServe and llm-d

This guide walks through deploying **RedHatAI/gpt-oss-20b** on Kubernetes using [KServe](https://kserve.github.io/website/). Steps are ordered from cluster setup through inference, AI gateway routing, optional prefix caching, and monitoring.

There are 3 alternate deployments detailed here:

1. default - a deployment of intelligent inference scheduling with vLLM and the llm-d scheduler
2. precise prefix cache aware routing - an advanced configuration that takes advantage of vLLM KV-Events
3. prefill-decode disaggregation - an advanced configuration that uses separate vLLM pods for the prefill and the decode stages of inference.

---

## Prerequisites

Meet the [LLMInferenceService minimum requirements](https://kserve.github.io/website/docs/next/admin-guide/kubernetes-deployment-llmisvc#minimum-requirements):

- **Kubernetes**: 1.32+
- **Cert Manager**: 1.18.0+
- **Gateway API**: 1.3.0+
- **Gateway API Inference Extension (GIE)**: 1.2.0
- **Gateway provider**: Envoy Gateway v1.5.0+
- **LeaderWorkerSet**: 0.6.2+ (for multi-node deployments)
- `kubectl` configured for your cluster, **cluster admin** permissions, **helm** v3+

For this guide you also need:

- Kubernetes cluster with GPU nodes (`nvidia.com/gpu`)
- KServe and LLM Inference Service CRDs installed (see [Kubernetes Deployment - LLMIsvc](https://kserve.github.io/website/docs/next/admin-guide/kubernetes-deployment-llmisvc))
- [Hugging Face](https://huggingface.co/) account and token for `RedHatAI/gpt-oss-20b`
- Optional: Envoy-based [Gateway API](https://gateway-api.sigs.k8s.io/) and AIGatewayRoute for the AI gateway
- Optional: Prometheus + Grafana for metrics

---

## 1. Create namespace

Create a dedicated namespace for the deployment (e.g. `kserve-lab`):

```bash
kubectl create namespace kserve-lab
```

All following resources use this namespace unless noted.

---

## 2. Persistent Volume (Claim) for model weights

Model weights are stored on a PersistentVolumeClaim so they can be reused by inference pods.

**Option A: Use the provided PVC**

The sample [model-pvc.yaml](./model-pvc.yaml) requests 256Gi with `ReadWriteMany`.

> Ensure your cluster has a default `StorageClass`, or define a `storageClass` in this file. If a default storageClass is not set or your storageClass does not support `ReadWriteMany` then the  initialization job dependent on this PVC will stay in pending state.

```bash
kubectl apply -f model-pvc.yaml -n kserve-lab
```

**Option B: Create a PV and PVC (e.g. NFS or local-path)**

If you use a storage class that requires a pre-created PV (e.g. some `local-path` setups), create a PersistentVolume first that matches the PVC’s size, access mode, and storage class, then apply the PVC.

Verify the PVC is bound:

```bash
kubectl get pvc -n kserve-lab
```

---

## 3. Hugging Face token secret

The model download job and (for prefix caching) the inference scheduler needs a [Hugging Face token](https://huggingface.co/docs/hub/en/security-tokens). We store this as an opaque secret with the key `HF_TOKEN`.

Set this as an environment variable:
```bash
export YOUR_HF_TOKEN=<your token from huggingface>
```

```bash
# Encode your Hugging Face token (replace YOUR_HF_TOKEN with your actual token)
kubectl create secret generic hf-token \
  --from-literal=HF_TOKEN="$YOUR_HF_TOKEN" \
  -n kserve-lab
```

Or apply [hf-token-secret.yaml](./hf-token-secret.yaml) after replacing the placeholder: set `stringData.HF_TOKEN` to your token.
---

## 4. Job to download model weights

Run a one-off Job that uses the PVC and a job to download **RedHatAI/gpt-oss-20b** into the shared volume:

```bash
kubectl apply -f model_weights_job.yaml -n kserve-lab
```

Wait for the Job to complete:

```bash
kubectl wait --for=condition=complete job/gpt-oss-20b-init-job -n kserve-lab --timeout=1h
# Or watch pods
kubectl get pods -n kserve-lab -l job-name=gpt-oss-20b-init-job -w
```

The Job uses [kserve-storage-initializer](https://github.com/kserve/kserve/tree/master/python/kserve_storage_initializer) and writes to the PVC at `/mnt/models`. Do not delete the PVC before inference is no longer needed.

---

## 5. LLMInferenceServiceConfig (template)

LLMInferenceServiceConfig defines the **pod and router template** (containers, volumes, scheduler, probes). The actual inference service will reference this by name.

### 6.1 Default config (intelligent inference scheduling)

Apply the default config:

```bash
kubectl apply -f llmisvc_config_default.yaml -n kserve-lab
```

This creates `LLMInferenceServiceConfig` named `llmisvc-intelligent-inference-scheduling`. It includes:

- vLLM server container serving `/mnt/models`
- Router with scheduler pointing at the EPP service

### 6.2 Use this config in the Inference Service

The Inference Service (next step) references this config via `baseRefs` (e.g. `llmisvc-intelligent-inference-scheduling`).

---

## 6. LLMInferenceService (inference service)

Create the LLMInferenceService that uses the downloaded model and the config template.

Apply the **default** inference service (no prefix caching):

```bash
kubectl apply -f inference_default.yaml -n kserve-lab
```

[inference_default.yaml](./inference_default.yaml) defines:

- **model**: `uri: "pvc://gpt-oss-20b-pvc"`, `name: RedHatAI/gpt-oss-20b`
- **replicas**: 2
- **baseRefs**: `llmisvc-intelligent-inference-scheduling`
- **resources**: adjust CPU, memory, and GPU (`nvidia.com/gpu`) to match your nodes

Wait until the service is ready (pods running, scheduler and vLLM ready):

```bash
kubectl get llminferenceservice -n kserve-lab
kubectl get pods -n kserve-lab -l app.kubernetes.io/name=gpt-oss-20b
```

---

## 7. AI Gateway and Route

To expose the model behind an AI gateway and route by model name (e.g. for OpenAI-compatible clients), create a Gateway and an AIGatewayRoute.

### 7.1 Gateway

```bash
kubectl apply -f gateway.yaml -n kserve-lab
```

[gateway.yaml](./gateway.yaml) defines an Envoy-based `Gateway` `ai-gateway` with an HTTP listener on port 80. Ensure the cluster has a Gateway controller that implements `gatewayClassName: envoy` (e.g. Envoy Gateway with AIGatewayRoute support).

### 7.2 AIGatewayRoute

```bash
kubectl apply -f ai-gateway-route.yaml -n kserve-lab
```

[ai-gateway-route.yaml](./ai-gateway-route.yaml) defines an `AIGatewayRoute` that:

- Attaches to `ai-gateway`
- Matches requests with header `x-ai-eg-model: RedHatAI/gpt-oss-20b`
- Backends to the LLMInferenceService’s `InferencePool`: `gpt-oss-20b-inference-pool`
- Optionally tracks token usage via `llmRequestCosts`

Clients should send the header `x-ai-eg-model: RedHatAI/gpt-oss-20b` when calling the gateway.

---

## 8. Alternate configuration: Precise Prefix Cache Aware Routing

For **precise prefix cache aware routing** (vLLM + EPP prefix indexer), use the dedicated config and inference service that enable KV cache events and the precise-prefix-cache-scorer. See [llm-d guide](https://llm-d.ai/docs/guide/Installation/precise-prefix-cache-aware) for an in-depth description.

### 8.1 LLMInferenceServiceConfig with precise prefix cache aware routing

```bash
kubectl apply -f llmisvc_config_prefix_cache.yaml -n kserve-lab
```

This creates `llmisvc-prefix-caching`. It adds:

- vLLM args: `--prefix-caching-hash-algo sha256_cbor`, `--block-size 64`, `--kv_transfer_config`, `--kv-events-config` (ZMQ to EPP)
- Router scheduler with `precise-prefix-cache-scorer`, `queue-scorer`, `kv-cache-utilization-scorer`, and tokenizers (HF) for the indexer
- Scheduler needs `hf-token` secret for tokenizer download (already created above)

### 8.2 Switch Inference Service to precise prefix cache aware routing

Either:

- **Replace** the default inference service by applying the prefix-cache variant (same name `gpt-oss-20b`, different `baseRefs`):

  ```bash
  kubectl apply -f inference_prefix_cache.yaml -n kserve-lab
  ```

- Or in [kustomization.yaml](./kustomization.yaml), comment out `inference_default.yaml` and uncomment `inference_prefix_cache.yaml`, then run `kubectl apply -k . -n kserve-lab`.

The Gateway and Route from step 7 still apply; they reference the same `InferencePool` name.

---

## 9. Alternative configuration: Prefill/Decode Disaggregation

For **prefill/decode disaggregation** (Prefill vLLM + Decode vLLM + EPP), use the alternative inference service that enables
Prefill and Decode to be performed on separate GPUs. See [llm-d guide](https://llm-d.ai/docs/guide/Installation/pd-disaggregation)
for an in depth description.

### 9.1 LLMInferenceServiceConfig with prefill/decode disaggregation

```bash
  kubectl apply -f llmisvc_config_pd_disagg.yaml -n kserve-lab
```

This creates `llmisvc-config-pd-disagg`. It adds:

- Decode container with vLLM args: `--kv_transfer_config '{\"kv_connector\":\"NixlConnector\",\"kv_role\":\"kv_both\"}'"`
- Prefill container with 2 replicas and same vLLM args 
- Scheduler needs `hf-token` secret for tokenizer download (already created above)

### 9.2 Switch Inference to prefill/decode disaggregation

> Requires 3 Nvidia GPUs. Reduce replica count or change to MIG partitions (see customization notes below) to run on less hardware.

Either:

- **Replace** the default inference service by applying the prefill-decode variant (same name `gpt-oss-20b`):

  ```bash
  kubectl apply -f inference_pd_disaggregation.yaml -n kserve-lab
  ```

- Or in [kustomization.yaml](./kustomization.yaml), comment out `inference_default.yaml` and uncomment `inference_pd_disaggregation.yaml`, then run `kubectl apply -k . -n kserve-lab`.

The Gateway and Route from step 7 still apply; they reference the same `InferencePool` name.

---


## 10. ServiceMonitor for Prometheus

To scrape vLLM and EPP metrics, apply the ServiceMonitor in the same namespace (or adjust `namespaceSelector` to your Prometheus setup):

1. Set `namespaceSelector.matchNames` in [service_monitor.yaml](./service_monitor.yaml) to your namespace (e.g. `[kserve-lab]`) so Prometheus discovers the targets.
2. Set `metadata.labels.release` to match the installation release name of the Prometheus stack installed on your system.

```bash
kubectl apply -f service_monitor.yaml -n kserve-lab
```

---

## 11. Grafana dashboards and example screenshots

KServe EPP metrics and llm-d observability are documented in the [grafana/](./grafana/) folder. Use these dashboards to monitor routing, prefix caching, and P/D disaggregation.

### 11.1 Dashboard files and links

| Dashboard | File | Description |
|-----------|------|-------------|
| **Kserve EPP – All** | [kserve-epp-all-dashboard.json](./grafana/kserve-epp-all-dashboard.json) | All-in-one: routing, prefix caching, P/D disaggregation |
| **Routing & Load Balancing** | [routing-load-balancing-dashboard.json](./grafana/routing-load-balancing-dashboard.json) | Request/token distribution, idle GPU time, routing latency |
| **Prefix Caching** | [prefix-caching-dashboard.json](./grafana/prefix-caching-dashboard.json) | vLLM prefix cache and EPP prefix indexer |
| **P/D Disaggregation** | [pd-disaggregation-dashboard.json](./grafana/pd-disaggregation-dashboard.json) | Prefill/decode workers, queue length, P/D decision rates |

Import in Grafana: **Dashboards** → **New** → **Import** → upload the JSON and select your Prometheus datasource. See [grafana/README.md](./grafana/README.md) for details and metric names (e.g. `vllm:kv_cache_usage_perc`, `--kv-cache-usage-percentage-metric`).

---

## Apply order (summary)

1. `kubectl create namespace kserve-lab`
1. `kubectl apply -f model-pvc.yaml -n kserve-lab`
1. `kubectl create secret generic hf-token --from-literal=HF_TOKEN="$YOUR_HF_TOKEN" -n kserve-lab`
1. `kubectl apply -f model_weights_job.yaml -n kserve-lab` → wait for completion
1. `kubectl apply -f llmisvc_config_default.yaml -n kserve-lab`
1. `kubectl apply -f inference_default.yaml -n kserve-lab`
1. `kubectl apply -f gateway.yaml -n kserve-lab` and `kubectl apply -f ai-gateway-route.yaml -n kserve-lab`
1. (Alternative) Prefix caching: `llmisvc_config_prefix_cache.yaml` then `inference_prefix_cache.yaml`
1. (Alternative) Prefill Decode Disaggregation: `inference_pd_disaggregation.yaml`
1. (Optional) `service_monitor.yaml` for Prometheus
1. (Optional) Import Grafana dashboards

Or use Kustomize from this directory (after fixing the HF token in the secret and choosing default vs prefix-cache inference):

```bash
kubectl apply -k docs/samples/llmisvc/e2e-gpt-oss/ -n kserve-lab
```

---

## Customization notes

- **GPU resource**: Inference YAMLs use `nvidia.com/gpu`. Replace with your GPU resource name if different (e.g. MIG `nvidia.com/mig-3g.40gb`).
- **Storage class**: If your cluster does not have `local-path`, change `storageClassName` in the PVC or create a matching PV.

For more on llm-d and monitoring, see [llm-d Observability and Monitoring](https://github.com/llm-d/llm-d/blob/main/docs/monitoring/README.md).
