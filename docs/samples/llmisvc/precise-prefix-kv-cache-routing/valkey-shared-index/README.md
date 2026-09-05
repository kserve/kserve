# Valkey-Backed Shared Prefix-Cache Index

Optional variant of the [precise prefix cache routing](../) example that backs the
`precise-prefix-cache-scorer`'s KV-block index with a shared [Valkey](https://valkey.io/)
instance instead of the Endpoint Picker's default in-memory index.

## Why

Each Endpoint Picker (EPP) replica normally tracks its own in-memory index of which KV-cache
blocks live on which pods. With multiple EPP replicas (HA deployments, large fleets), each
replica only sees the KV-cache events it happened to receive, so the index fragments across
replicas and routing decisions become less accurate. Pointing all EPP replicas at the same
Valkey instance gives them a single, consistent view of the cluster-wide KV-cache state.

A single EPP replica has nothing to share, so the in-memory default is the better choice there.

## What this deploys

[`llm-inference-service-qwen2-7b-gpu-kv-cache-routing-valkey.yaml`](llm-inference-service-qwen2-7b-gpu-kv-cache-routing-valkey.yaml)
is the same `LLMInferenceService` as the parent example, with one change: the
`precise-prefix-cache-scorer`'s `indexerConfig.kvBlockIndexConfig` now sets `valkeyConfig`
instead of relying on the default in-memory index. It also includes a single-replica
`valkey/valkey:9.1.0` `Deployment` and `Service` for the index to connect to.

```yaml
indexerConfig:
  kvBlockIndexConfig:
    valkeyConfig:
      address: "valkey://valkey:6379"
      backendType: "valkey"
    enableMetrics: true
    metricsLoggingInterval: 60000000000
```

This is a pure configuration change backed by [`llm-d-kv-cache`](https://github.com/llm-d/llm-d-kv-cache)'s
`ValkeyConfig` / `NewValkeyIndex()`, which is API-compatible with its Redis index (`RedisIndexConfig`
is reused for both backends).

## Deploy

See the parent [README](../README.md) for prerequisites and the full deployment/verification
walkthrough. Apply this variant instead of the parent manifest:

```bash
kubectl apply -f llm-inference-service-qwen2-7b-gpu-kv-cache-routing-valkey.yaml
```
