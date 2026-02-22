# Precise Prefix KV Cache Routing

This directory contains an example configuration demonstrating advanced KV cache routing with precise prefix matching to optimize inference performance by routing requests to instances with matching cached content.

## Overview

This example showcases how to configure the scheduler to track KV cache blocks across inference endpoints and route requests to the endpoint with the highest cache hit rate. This significantly improves throughput and reduces latency by maximizing cache reuse.

## Prerequisites

- Kubernetes cluster with GPU nodes
- Model weights accessible via HuggingFace or PVC
- Understanding of vLLM's prefix caching mechanism

## Example

### KV Cache Routing with Qwen2.5-7B ([`llm-inference-service-qwen2-7b-gpu-kv-cache-routing.yaml`](llm-inference-service-qwen2-7b-gpu-kv-cache-routing.yaml))

This example demonstrates precise prefix cache routing with cache block tracking.

**Configuration:**

- Model: Qwen2.5-7B-Instruct
- Replicas: 2
- GPU per replica: 1
- Prefix caching algorithm: SHA256 CBOR 64-bit
- Block size: 64 tokens
- KV cache tracking: Enabled via ZMQ

**Key Features:**

- **Prefix Cache Scorer**: Routes requests to endpoints with matching KV cache blocks (weight: 2.0)
- **Load-Aware Scorer**: Balances load across endpoints (weight: 1.0)
- **Cache Tracking Mode**: Real-time tracking of KV cache blocks using ZMQ events
- **KV Transfer**: Enabled via NixlConnector for cache sharing between instances
- **Block Size**: 64 tokens (configurable, must match between vLLM and scheduler)
- **Hash Seed**: PYTHONHASHSEED=42 (must match across all components)

## How It Works

1. **Cache Event Publishing**: Each vLLM instance publishes KV cache events via ZMQ whenever cache blocks are created or evicted
2. **Cache Tracking**: The scheduler maintains an index of which cache blocks are present on which endpoints
3. **Request Routing**: When a request arrives, the scheduler:
   - Computes the prefix hash using the same algorithm as vLLM
   - Queries the cache index to find endpoints with matching blocks
   - Scores endpoints based on cache hit potential (weight 2.0) and current load (weight 1.0)
   - Routes to the endpoint with the highest combined score

## Scheduler Configuration

The example uses a custom scheduler configuration with the following plugins:

- **single-profile-handler**: Single scheduling profile for all requests
- **prefix-cache-scorer**:
  - Mode: `cache_tracking` (real-time tracking via ZMQ)
  - Block size: 64 tokens (must match vLLM `--block-size`)
  - Hash seed: 42 (must match `PYTHONHASHSEED`)
  - Metrics enabled with 60-second logging interval
- **load-aware-scorer**: Balances load across endpoints
- **max-score-picker**: Selects endpoint with highest combined score

## vLLM Configuration

Key vLLM settings for cache routing:

```yaml
VLLM_ADDITIONAL_ARGS:
  - --prefix-caching-hash-algo sha256_cbor_64bit
  - --block-size 64
  - --kv_transfer_config '{"kv_connector":"NixlConnector","kv_role":"kv_both"}'
  - --kv-events-config '{"enable_kv_cache_events":true,"publisher":"zmq","endpoint":"tcp://...:5557","topic":"kv@${POD_IP}@..."}'

PYTHONHASHSEED: "42"
```

## Important Parameters

### Block Size

The block size must match between vLLM and the scheduler. Common values:
- Default: 16 tokens
- This example: 64 tokens (better for longer sequences)
- Larger blocks = fewer cache blocks, but less granular matching

### Hash Seed

`PYTHONHASHSEED` must be consistent across:
- All vLLM pods
- Scheduler configuration
- Any tools computing cache hashes

Mismatch will result in cache misses even for identical prefixes.

### Cache Event Endpoint

The ZMQ endpoint format:
```
tcp://{{ ChildName .ObjectMeta.Name `-epp-service` }}:5557
```

This uses a Kubernetes template to reference the event publisher service created by the operator.

## Resource Requirements

Each replica requires:
- 1 NVIDIA GPU
- 4 CPU cores (limit), 2 cores (request)
- 32Gi memory (limit), 16Gi memory (request)

## Deployment

1. Deploy the LLMInferenceService:
   ```bash
   kubectl apply -f llm-inference-service-qwen2-7b-gpu-kv-cache-routing.yaml
   ```

2. Monitor cache routing effectiveness:
   ```bash
   # Check scheduler metrics for cache hit rates
   kubectl logs -l app.kubernetes.io/component=llminferenceservice-scheduler -f

   # View KV cache index metrics
   # Look for metrics like kv_block_index_size, cache_hit_rate, etc.
   ```

3. Test with repeated prompts to see cache routing in action:
   ```bash
   # Send requests with common prefixes
   # The scheduler should route similar requests to the same endpoint
   ```

## Benefits

- **Reduced Latency**: Cache hits eliminate the need to recompute attention for cached tokens
- **Higher Throughput**: More requests can be processed with the same GPU resources
- **Efficient Resource Usage**: Better distribution of workload based on cache state
- **Automatic Optimization**: No manual routing rules needed

## Monitoring

Key metrics to monitor:

- **Cache hit rate**: Percentage of tokens served from cache
- **KV block index size**: Number of tracked cache blocks
- **Endpoint scores**: How the scheduler scores each endpoint
- **Request distribution**: Whether similar requests are routed to the same endpoint

## Tuning Tips

1. **Increase block size** for longer sequences to reduce index overhead
2. **Adjust scorer weights** to prioritize cache hits over load balancing (or vice versa)
3. **Enable detailed metrics** during testing to understand routing behavior
4. **Monitor cache eviction** patterns to optimize cache size settings