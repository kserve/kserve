# 00 - Existing LoRA Support

This directory covers LoRA adapter serving with the capabilities available in the
product today — using `VLLM_ADDITIONAL_ARGS` to pass LoRA flags directly to vLLM.
The controller has no awareness of LoRA configuration in this mode.

Both approaches let you serve multiple fine-tuned LoRA adapters from a single
base model — clients select which adapter to use via the `model` field in the
OpenAI-compatible API.

## Two approaches

### [00_static_lora/](00_static_lora/) — Static adapter configuration

Adapters are pre-declared via `--lora-modules` in `VLLM_ADDITIONAL_ARGS`. vLLM
downloads and loads them at startup. Simple, but any adapter change requires
re-rolling the deployment.

**Best for:** Getting started, stable set of adapters that rarely change.

### [01_dynamic_lora/](01_dynamic_lora/) — Dynamic adapter management

The base model starts with `--enable-lora` but no pre-loaded adapters. In-cluster
Jobs call vLLM's `/v1/load_lora_adapter` and `/v1/unload_lora_adapter` endpoints
to manage adapters at runtime — no restarts needed.

**Best for:** Frequently changing adapters, multi-tenant environments where
different teams manage their own adapters.

## LoRA-aware routing

Both approaches benefit from the Gateway API Inference Extension's **LoRA affinity
scorer**, which routes requests to pods where the requested adapter is already
loaded. This maximizes adapter cache reuse and minimizes cold-load latency with
no additional configuration.

| Score | Condition |
|-------|-----------|
| 1.0   | Requested adapter is already active on the endpoint |
| 0.8   | Adapter not active but endpoint has capacity to load it |
| 0.6   | Adapter is queued/waiting to be loaded |
| 0.0   | Endpoint is full and adapter is neither active nor queued |

## What's next

| Directory | Description |
|-----------|-------------|
| `01_declarative_lora/` | First-class `model.lora.adapters` API with controller-managed storage and flag injection |
| `02_production_hardening/` | Rate limiting, auth, and quota per adapter via Red Hat Connectivity Link |
| `03_multi_tenant_isolation/` | Namespace isolation with cross-namespace adapter management |
| `04_scheduled_adapter_rotation/` | CronJob-based adapter swaps (e.g., daytime vs. nighttime workloads) |
| `05_per_adapter_observability/` | Prometheus alerts and Grafana dashboards for per-adapter metrics |
| `06_secure_adapter_supply_chain/` | Model signing and scanning for adapter weight verification |
