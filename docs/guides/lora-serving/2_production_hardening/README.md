# 02 - Production Hardening with Red Hat Connectivity Link

This phase adds enterprise gateway controls to the LoRA serving setup using
[Red Hat Connectivity Link](https://docs.redhat.com/en/documentation/red_hat_connectivity_link)
(upstream: [Kuadrant](https://kuadrant.io/)).

All policies use the **Kuadrant v1 stable API** (`apiVersion: kuadrant.io/v1`)
and attach to Gateway API resources (HTTPRoute or Gateway) via `targetRef`.

## Prerequisites

- LoRA serving deployed and working (see `0_existing_lora_support/` or `1_declarative_lora/`)
- Red Hat Connectivity Link operator installed (provides Kuadrant, Authorino, Limitador)
- cert-manager operator installed (for TLSPolicy)
- Gateway API CRDs installed

## Architecture

```
External Client
    │
    ▼
┌─────────────────────────────┐
│  Red Hat Connectivity Link  │
│  ┌───────────────────────┐  │
│  │ TLSPolicy  → Gateway  │  │
│  │ AuthPolicy → HTTPRoute│  │
│  │ RateLimitPolicy       │  │
│  │            → HTTPRoute│  │
│  └───────────────────────┘  │
└──────────┬──────────────────┘
           │
           ▼
┌─────────────────────────────┐
│  Gateway (Istio/Envoy)      │
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

## Manifests

| File | Resource | Attaches To | Purpose |
|------|----------|-------------|---------|
| `tls-policy.yaml` | TLSPolicy | Gateway | TLS termination via cert-manager |
| `cluster-issuer.yaml` | ClusterIssuer | - | cert-manager issuer for TLS certs |
| `auth-policy.yaml` | AuthPolicy | HTTPRoute | API key authentication |
| `api-key-secrets.yaml` | Secret (x3) | - | API key credentials per team |
| `rate-limit-policy.yaml` | RateLimitPolicy | HTTPRoute | Per-adapter and per-user rate limits (body-based) |
| `rate-limit-policy-header-based.yaml` | RateLimitPolicy | HTTPRoute | Alternative: header-based rate limits |
| `kustomization.yaml` | Kustomization | - | Deploys all resources |

## Key Design Decision: Per-Adapter Rate Limiting

The central challenge is that the LoRA adapter name lives in the **request body**,
not the URL path. OpenAI-compatible requests look like:

```json
{
  "model": "finance-lora",
  "messages": [{"role": "user", "content": "..."}]
}
```

### Option A: Request Body Matching (Recommended)

Kuadrant's wasm-shim provides `requestBodyJSON()`, a CEL function that parses
the request body using [RFC 6901 JSON Pointer](https://datatracker.ietf.org/doc/html/rfc6901)
syntax:

```yaml
when:
  - predicate: "requestBodyJSON('/model') == 'finance-lora'"
```

**Requirement**: Envoy must be configured to buffer request bodies. This is
disabled by default. Consult your gateway provider's documentation for enabling
`with_request_body` on the external authorization filter, or use an EnvoyFilter.

### Option B: Header-Based Matching (Fallback)

If body buffering is unavailable, use a header-based approach:

```yaml
when:
  - predicate: "request.headers['x-model-name'] == 'finance-lora'"
```

This requires either:
- The Gateway API Inference Extension to set an `x-model-name` header
  (it already parses the body for routing)
- An EnvoyFilter/Lua filter that extracts the model name into a header
- Clients to send a custom header alongside the body

See `rate-limit-policy-header-based.yaml` for the full manifest.

## Deployment

```bash
# 1. Edit placeholder values
#    - REPLACE_ME_NAMESPACE in kustomization.yaml
#    - REPLACE_ME_GATEWAY_NAME / REPLACE_ME_GATEWAY_NAMESPACE in tls-policy.yaml
#    - REPLACE_ME_CLUSTER_ISSUER_NAME in tls-policy.yaml
#    - API key values in api-key-secrets.yaml

# 2. Apply
kubectl apply -k .

# 3. Verify policies are accepted
kubectl get authpolicy,ratelimitpolicy,tlspolicy
```

## Testing

```bash
GATEWAY_URL="https://your-gateway-host"

# Unauthenticated request (should be rejected 401)
curl -s -o /dev/null -w "%{http_code}" \
  "${GATEWAY_URL}/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -d '{"model": "k8s-lora", "messages": [{"role": "user", "content": "hello"}]}'

# Authenticated request
curl "${GATEWAY_URL}/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -H "Authorization: APIKEY REPLACE_ME_TEAM_ML_KEY" \
  -d '{"model": "k8s-lora", "messages": [{"role": "user", "content": "What is a Pod?"}]}'

# Exceed rate limit (run in a loop)
for i in $(seq 1 25); do
  curl -s -o /dev/null -w "%{http_code}\n" \
    "${GATEWAY_URL}/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -H "Authorization: APIKEY REPLACE_ME_DEV_SANDBOX_KEY" \
    -d '{"model": "k8s-lora", "messages": [{"role": "user", "content": "test"}]}'
done
# After 20 requests, standard-tier users should receive 429 Too Many Requests
```

## Rate Limit Structure

| Limit Name | Scope | Rate | Window | Condition |
|-----------|-------|------|--------|-----------|
| `global-limit` | All requests | 500 | 1 min | None (safety net) |
| `per-user-premium` | Per user | 100 | 1 min | `tier == 'premium'` |
| `per-user-standard` | Per user | 20 | 1 min | `tier == 'standard'` |
| `adapter-finance-lora` | All users | 200 | 1 min | `model == 'finance-lora'` |
| `adapter-k8s-lora` | All users | 100 | 1 min | `model == 'k8s-lora'` |
| `adapter-default` | Per adapter | 50 | 1 min | Any other model |

## Future: TokenRateLimitPolicy

Kuadrant v1.3+ introduces `TokenRateLimitPolicy` (`kuadrant.io/v1alpha1`),
which counts **LLM tokens** instead of requests by reading the
`usage.total_tokens` field from OpenAI-compatible responses. This is a better
fit for LLM cost management. When it reaches v1 stability, it can replace
or supplement the request-based RateLimitPolicy shown here.

## References

- [Kuadrant RateLimitPolicy Reference](https://docs.kuadrant.io/latest/kuadrant-operator/doc/reference/ratelimitpolicy/)
- [Kuadrant AuthPolicy Reference](https://docs.kuadrant.io/latest/kuadrant-operator/doc/reference/authpolicy/)
- [Kuadrant TLSPolicy Reference](https://docs.kuadrant.io/latest/kuadrant-operator/doc/reference/tlspolicy/)
- [Kuadrant Well-Known Attributes (RFC 0002)](https://github.com/Kuadrant/architecture/blob/main/rfcs/0002-well-known-attributes.md)
- [Kuadrant wasm-shim (requestBodyJSON)](https://github.com/Kuadrant/wasm-shim)
- [TokenRateLimitPolicy Blog Post](https://kuadrant.io/blog/token-rate-limiting/)
- [Authenticated Rate Limiting Guide](https://docs.kuadrant.io/1.3.x/kuadrant-operator/doc/user-guides/ratelimiting/authenticated-rl-for-app-developers/)
