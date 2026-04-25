# 03 - Multi-Tenant Adapter Isolation

This directory demonstrates how multiple teams can share a single
LLMInferenceService (and its GPU resources) while maintaining strict
isolation over which LoRA adapters each team can manage.

## Architecture

```
┌───────────────────┐     ┌───────────────────┐
│  lora-team-a      │     │  lora-team-b      │
│  ┌─────────────┐  │     │  ┌─────────────┐  │
│  │ lora-manager │  │     │  │ lora-manager │  │
│  │ SA + RBAC   │  │     │  │ SA + RBAC   │  │
│  └──────┬──────┘  │     │  └──────┬──────┘  │
│         │ load/   │     │         │ load/   │
│         │ unload  │     │         │ unload  │
└─────────┼─────────┘     └─────────┼─────────┘
          │                         │
          ▼                         ▼
┌─────────────────────────────────────────────┐
│  lora-serving (shared namespace)            │
│                                             │
│  LLMInferenceService: qwen2-lora            │
│  ├─ team-a-finance-lora  (loaded by Team A) │
│  ├─ team-a-code-lora     (loaded by Team A) │
│  ├─ team-b-support-lora  (loaded by Team B) │
│  └─ base model always available             │
│                                             │
│  NetworkPolicy: only tenant namespaces      │
│  can reach vLLM pods                        │
└─────────────────────────────────────────────┘
```

## Isolation model

| Boundary | Mechanism | Effect |
|----------|-----------|--------|
| **Who can manage adapters** | RBAC (per-namespace ServiceAccount) | Team A's Jobs run as `lora-team-a/lora-manager`, Team B's as `lora-team-b/lora-manager` |
| **Network access** | NetworkPolicy | Only labeled tenant namespaces can reach vLLM pods |
| **Audit trail** | Kubernetes audit logs | Every adapter load/unload is tied to a specific ServiceAccount and namespace |
| **Adapter naming** | Convention (`team-a-*`, `team-b-*`) | Prevents accidental collisions; enforceable via admission webhooks |

## What's included

| File | Purpose |
|------|---------|
| `shared-model-namespace.yaml` | Namespace for the LLMInferenceService |
| `team-a-namespace.yaml` | Team A's namespace |
| `team-b-namespace.yaml` | Team B's namespace |
| `rbac-team-a.yaml` | SA, Role, RoleBinding for Team A |
| `rbac-team-b.yaml` | SA, Role, RoleBinding for Team B |
| `network-policy.yaml` | Restricts vLLM pod access to tenant namespaces |
| `lora-load-job-team-a.yaml` | Example: Team A loads an adapter |
| `kustomization.yaml` | Deploys the multi-tenant infrastructure |

## Deployment

### 1. Create the namespaces and RBAC

```bash
kubectl apply -k .
```

### 2. Deploy the LLMInferenceService in the shared namespace

Use the LLMInferenceService from `00_existing_lora_support/01_dynamic_lora/` deployed
into the `lora-serving` namespace.

### 3. Team A loads their adapter

```bash
kubectl apply -f lora-load-job-team-a.yaml
kubectl logs -n lora-team-a job/load-team-a-adapter
```

### 4. Team B tries to load an adapter

Team B runs a similar Job from `lora-team-b` namespace using their own
`lora-manager` ServiceAccount.

### 5. Verify isolation

```bash
# Team A can see pods in lora-serving (cross-namespace RoleBinding)
kubectl auth can-i get pods -n lora-serving \
  --as=system:serviceaccount:lora-team-a:lora-manager
# yes

# Team A cannot see pods in Team B's namespace
kubectl auth can-i get pods -n lora-team-b \
  --as=system:serviceaccount:lora-team-a:lora-manager
# no
```

## Adapter naming convention

Teams prefix their adapter names with their team identifier:
- Team A: `team-a-finance-lora`, `team-a-code-lora`
- Team B: `team-b-support-lora`, `team-b-translation-lora`

This convention is enforced socially (naming convention) or technically
(via an OPA/Gatekeeper policy or admission webhook that validates adapter
names in load requests match the calling namespace's team label).

## Scaling to more teams

Adding a new team requires:
1. A new namespace YAML (copy `team-a-namespace.yaml`, change labels)
2. A new RBAC file (copy `rbac-team-a.yaml`, change namespace references)
3. Add the namespace to the NetworkPolicy's `namespaceSelector`
