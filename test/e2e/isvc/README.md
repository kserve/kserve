# InferenceService Tracing E2E Tests

These tests verify tracing for raw, Standard-mode InferenceServices. They use
Jaeger as the OTLP receiver and query its API through the Kubernetes service
proxy.

## Coverage

The `tracing` marker selects four cases:

1. A canary request records the selected predictor variant.
2. A custom transformer forwards a KServe V1 prompt to native vLLM, and the
   transformer and predictor spans share a trace with a `CHILD_OF` path.
3. An InferenceService with omitted `spec.tracing` serves successfully without
   OpenTelemetry environment variables, response headers, or Jaeger spans.
4. An inbound sampled `traceparent` is preserved through the transformer and
   predictor, including the parent reference on every downstream span.

The trace graph assertions also have pure tests covering disconnected traces,
shared trace IDs without lineage, and valid parent and transformer lineage.

## Requirements

- A raw KServe installation with the Jaeger observability service available.
- Gateway API for the canary case. The CI job uses `envoy-gatewayapi`.
- A CPU-capable cluster with enough capacity for the pinned
  `vllm/vllm-openai-cpu:v0.19.0` image and its `facebook/opt-125m` model.
- The built custom transformer image referenced by `IMAGE_TRANSFORMER_IMG_TAG`.
  CI loads this image, the vLLM image, and an OCI modelcar containing
  `facebook/opt-125m` before running the tests.

## Running

From `test/e2e/`:

```bash
pytest -m tracing isvc/ --network-layer=envoy-gatewayapi
pytest isvc/test_tracing_assertions.py
```

The model URI defaults to `hf://facebook/opt-125m`. Set
`OPT_125M_OCI_MODEL_URI` when using a preloaded OCI modelcar, as the CI job
does.
