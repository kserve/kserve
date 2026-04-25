# 02 - Scheduled Adapter Rotation

This directory demonstrates time-based LoRA adapter rotation. Different
adapters are loaded during business hours vs. off-hours, optimizing GPU
memory allocation for different workload patterns without restarting the
model server.

## Use case

A platform team serves two workload patterns on the same GPU infrastructure:

| Time | Workload | Adapter | Optimization |
|------|----------|---------|-------------|
| 8 AM - 8 PM (weekdays) | Interactive user queries | `interactive-lora` | Low latency, high concurrency |
| 8 PM - 8 AM (nights/weekends) | Batch document processing | `batch-processing-lora` | High throughput, large context |

Loading both adapters simultaneously would waste GPU memory. Rotating them
on a schedule lets each workload use the full GPU memory budget.

## What's included

| File | Purpose |
|------|---------|
| `llm-inference-service-lora.yaml` | Base model with `--enable-lora`, no static adapters |
| `rbac.yaml` | ServiceAccount, Role, and RoleBinding for adapter management |
| `lora-swap-job.yaml` | One-shot Job to manually trigger an adapter swap |
| `cronjob-daytime.yaml` | CronJob: loads interactive adapter at 8 AM UTC weekdays |
| `cronjob-nighttime.yaml` | CronJob: loads batch adapter at 8 PM UTC weekdays |
| `kustomization.yaml` | Deploys the infrastructure and CronJobs |

## Deployment

### 1. Deploy the base infrastructure

```bash
# Edit kustomization.yaml to set your namespace
# Edit REPLACE_ME_HF_REPO in the Job/CronJob YAMLs with your adapter repos

kubectl apply -k .
```

### 2. Wait for the model server to be ready

```bash
kubectl wait --for=condition=Ready llminferenceservice/qwen2-lora --timeout=600s
```

### 3. Test a manual swap

Before enabling the CronJobs, trigger a swap manually to verify it works:

```bash
# Load the interactive adapter (and unload batch if present)
kubectl apply -f lora-swap-job.yaml
kubectl logs -f job/lora-swap

# Verify the adapter is loaded
curl -k https://<route-url>/v1/models | jq '.data[].id'

# Test inference with the swapped adapter
curl -k https://<route-url>/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "interactive-lora",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

To swap to a different adapter, delete the completed Job and re-apply with
new values:

```bash
kubectl delete job lora-swap
# Edit lora-swap-job.yaml: change UNLOAD_ADAPTER, LOAD_ADAPTER, LOAD_ADAPTER_SOURCE
kubectl apply -f lora-swap-job.yaml
```

### 4. Enable scheduled rotation

Once the manual swap is verified, the CronJobs handle it automatically:

```bash
# Check CronJob schedules
kubectl get cronjobs

# Manually trigger a CronJob to test the scheduled path
kubectl create job --from=cronjob/lora-rotate-daytime manual-daytime-test
kubectl logs job/manual-daytime-test

# Verify which adapters are loaded
curl -k https://<route-url>/v1/models | jq '.data[].id'
```

## How the CronJobs work

```
08:00 UTC Mon-Fri                    20:00 UTC Mon-Fri
┌────────────────┐                   ┌────────────────┐
│ lora-rotate-   │                   │ lora-rotate-   │
│ daytime        │                   │ nighttime      │
│                │                   │                │
│ 1. Unload      │                   │ 1. Unload      │
│    batch-*     │                   │    interactive-*│
│ 2. Load        │                   │ 2. Load        │
│    interactive-*│                  │    batch-*     │
└────────────────┘                   └────────────────┘
         │                                    │
         ▼                                    ▼
┌───────────────────────────────────────────────────┐
│ LLMInferenceService (qwen2-lora)                  │
│ vLLM --enable-lora                                │
│                                                   │
│ Day:   [base model] + [interactive-lora]           │
│ Night: [base model] + [batch-processing-lora]      │
└───────────────────────────────────────────────────┘
```

## Customization

### Change the schedule

Edit `spec.schedule` in the CronJob YAMLs. Uses standard cron syntax:

```yaml
# Every 6 hours
schedule: "0 */6 * * *"

# Weekdays at 9 AM EST (14:00 UTC)
schedule: "0 14 * * 1-5"

# First Monday of each month
schedule: "0 8 1-7 * 1"
```

### Add more rotation slots

Create additional CronJobs for more time slots (e.g., a weekend adapter,
a month-end reporting adapter). Each CronJob follows the same pattern:
unload the previous adapter, load the next one.

### Combine with multi-tenant isolation

Use the `lora-manager` ServiceAccount from a specific team namespace
(`future/3_multi_tenant_isolation/`) to restrict which teams can run rotations.
