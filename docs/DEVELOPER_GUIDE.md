# Developer Guide

Please review the [KServe Developer Guide](https://github.com/kserve/website/blob/main/docs/developer/developer.md) docs.

## Tracing headers

KServe's Python REST data plane automatically emits the active OpenTelemetry trace ID on every HTTP response once tracing is enabled. By default, the runtime surfaces a W3C Trace Context compliant `traceparent` header (and `tracestate` when available) so that downstream systems can correlate logs and spans. Operators can override the header names by setting the `TRACE_RESPONSE_HEADER_NAME` and `TRACE_RESPONSE_TRACESTATE_HEADER_NAME` environment variables on the serving runtime.

