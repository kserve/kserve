# 03 - Production Hardening with Red Hat Connectivity Link

> **Status: Coming soon**

This phase will demonstrate production-grade LoRA serving using
[Red Hat Connectivity Link](https://docs.redhat.com/en/documentation/red_hat_connectivity_link)
(downstream [Kuadrant](https://kuadrant.io/)) to add enterprise controls at the gateway layer.

## Planned Features

### Per-Adapter Rate Limiting
Apply different rate limits to different LoRA adapters based on business priority:
- High-priority adapters (e.g., `finance-lora`) get higher throughput limits
- Experimental or lower-priority adapters get conservative limits
- Prevents any single adapter from starving others

### Authentication & Authorization
- API key or OAuth-based authentication at the gateway
- Per-tenant access control to specific adapters
- Integration with OpenShift OAuth / RHSSO

### Quota Management
- Token-based quotas per tenant or per adapter
- Usage tracking and reporting
- Graceful degradation when quotas are exceeded

## Prerequisites

- Red Hat Connectivity Link operator installed via OperatorHub
- Gateway API resources configured

## Architecture

```
External Client
    │
    ▼
┌─────────────────────────────┐
│  Red Hat Connectivity Link  │
│  ┌───────────────────────┐  │
│  │ AuthPolicy            │  │
│  │ RateLimitPolicy       │  │
│  │ TLSPolicy             │  │
│  └───────────────────────┘  │
└──────────┬──────────────────┘
           │
           ▼
┌─────────────────────────────┐
│  Gateway (RHOAI-managed)    │
│  HTTPRoute → InferencePool  │
└──────────┬──────────────────┘
           │
           ▼
┌─────────────────────────────┐
│  LLMInferenceService        │
│  (qwen2-lora)               │
│  ├─ k8s-lora                │
│  ├─ finance-lora            │
│  └─ (dynamic adapters)      │
└─────────────────────────────┘
```
