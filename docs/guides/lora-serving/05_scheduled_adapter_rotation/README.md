# 05 - Scheduled Adapter Rotation

This directory demonstrates time-based LoRA adapter rotation using CronJobs.
Different adapters are loaded during business hours vs. off-hours, optimizing
GPU memory allocation for different workload patterns without restarting the
model server.

## Use case

A platform team serves two workload patterns on the same GPU infrastructure:

| Time | Workload | Adapter | Optimization |
|------|----------|---------|-------------|
| 8 AM - 8 PM (weekdays) | Interactive user queries | `interactive-lora` | Low latency, high concurrency |
| 8 PM - 8 AM (nights/weekends) | Batch document processing | `batch-processing-lora` | High throughput, large context |

Loading both adapters simultaneously would waste GPU memory. Rotating them
on a schedule lets each workload use the full GPU memory budget.

## How it works

Two CronJobs run the rotation:

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

## Prerequisites

This builds on `02_dynamic_lora_lifecycle/`. You need:
- The LLMInferenceService with `--enable-lora` deployed
- The `lora-manager` ServiceAccount and RBAC from `02_dynamic_lora_lifecycle/rbac.yaml`

## Deployment

```bash
# Edit kustomization.yaml to set your namespace
# Edit REPLACE_ME_HF_REPO in the CronJob YAMLs with your adapter repos

kubectl apply -k .
```

## Verification

```bash
# Check CronJob schedules
kubectl get cronjobs

# Manually trigger a rotation to test
kubectl create job --from=cronjob/lora-rotate-daytime manual-daytime-test
kubectl logs job/manual-daytime-test

# Verify which adapters are loaded
curl -k https://<route-url>/v1/models | jq '.data[].id'
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
(`04_multi_tenant_isolation/`) to restrict which teams can run rotations.
