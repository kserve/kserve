# Developer Guide

Please review the [KServe Developer Guide](https://github.com/kserve/website/blob/main/docs/developer-guide/index.md) docs.

## Tracing headers

KServe's Python REST data plane automatically emits the active OpenTelemetry trace ID on every HTTP response when tracing is configured. By default, the runtime surfaces a W3C Trace Context compliant `traceparent` header (and `tracestate` when available) so that downstream systems can correlate logs and spans. Operators can override the header names by setting the `TRACE_RESPONSE_HEADER_NAME` and `TRACE_RESPONSE_TRACESTATE_HEADER_NAME` environment variables on the serving runtime.

Instrumentation and span exporting are configured independently. Set `OTEL_TRACES_EXPORTER=otlp` to export spans using the standard `OTEL_EXPORTER_OTLP_*` settings, or set `OTEL_TRACES_EXPORTER=console` for local diagnostics. Set `OTEL_TRACES_EXPORTER=none` to keep instrumentation, context propagation, and response trace headers enabled without exporting spans. If neither `OTEL_TRACES_EXPORTER` nor an OTLP endpoint is configured, KServe does not create a tracer provider, instrument requests, or add response trace headers. Sampling follows the standard `OTEL_TRACES_SAMPLER` and `OTEL_TRACES_SAMPLER_ARG` settings. Set `OTEL_SDK_DISABLED=true` to disable tracing instrumentation entirely.
