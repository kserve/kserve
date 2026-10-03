# RHOAIENG-89172: distributed-tracing PR verification

Audit date: 2026-09-21

Scope: verify whether KServe PR [#6108](https://github.com/kserve/kserve/pull/6108) and PR [#6198](https://github.com/kserve/kserve/pull/6198) contain the implementation needed by the four e2e scenarios in [RHOAIENG-89172](https://redhat.atlassian.net/browse/RHOAIENG-89172).

This is a static source audit. No cluster or Jaeger e2e run was performed.

## Jira requirements

The Jira description lists these cases:

1. Connected trace: transformer plus vLLM predictor, `spec.tracing` with a 100% sampler, and transformer/predictor spans sharing one trace ID.
2. Canary variant: traffic routed to a canary predictor and the trace containing `isvc.predictor.variant`.
3. Disabled control: no injected OTEL variables and no KServe-created inference spans when `spec.tracing` is omitted.
4. Parent propagation: all downstream spans are children of an inbound `traceparent`.

It also requires reuse of `hack/setup/infra/manage.jaeger-helm.sh`, tests in `test/e2e/isvc/test_tracing.py`, all four cases passing in CI, and assertions that distinguish one connected trace from merely existing spans.

## Snapshot and attribution

The exact remote PR heads inspected were:

- PR #6108: `d7ef6326bd181cfa8cbdafda5dadff49050ae673`, whose unique change is the API/configuration surface. It was merged into the fetched master history as `7310e1f887bf0ef320df5a46784515485f6ad96a`.
- PR #6198: `fbbd5b49cd9e01bf749a9d44f517de02e056fedf`, based at `85991f1693d4713f498702f8e2b350a9ef520db2`. Its unique change is Python model-server tracing plus a predictor-level e2e test. It was merged into the fetched master history as `d2123b12536f04633bda3da9cd846a7e267e58fa`.

The controller-side injection code visible in the PR #6198 snapshot is not part of PR #6198's unique diff. Git history attributes the relevant pieces to separate changes: [#6172 / `102442c7`](https://github.com/kserve/kserve/commit/102442c78636c3374b1f0e1f76d7dddf8a6e67d5), [#6214 / `cee4cc06`](https://github.com/kserve/kserve/commit/cee4cc0612ad05619cba701bf22a0ba47baa18ed), and [#6217 / `82e8def4`](https://github.com/kserve/kserve/commit/82e8def4d31923f9626b8b88a805b1e3d68aad54). This distinction matters when determining what these two PRs themselves implemented.

## Implementation evidence

### PR #6108: API and admission support — implemented

- `InferenceServiceSpec` contains the optional pointer-valued `spec.tracing` field, so omitted tracing remains distinguishable from an empty tracing object: [`inference_service.go`](https://github.com/kserve/kserve/blob/d7ef6326bd181cfa8cbdafda5dadff49050ae673/pkg/apis/serving/v1beta1/inference_service.go#L23-L30).
- `TracingSpec` defines exporter endpoint, sampler, sampler argument, and exporter, with defaults for OTLP endpoint, `parentbased_traceidratio`, `0.05`, and `otlp`: [`tracing.go`](https://github.com/kserve/kserve/blob/d7ef6326bd181cfa8cbdafda5dadff49050ae673/pkg/apis/serving/v1beta1/tracing.go#L21-L70).
- Defaults are applied only when `spec.tracing` is non-nil: [`inference_service_defaults.go`](https://github.com/kserve/kserve/blob/d7ef6326bd181cfa8cbdafda5dadff49050ae673/pkg/apis/serving/v1beta1/inference_service_defaults.go#L163-L166).
- Admission validation covers supported sampler names and HTTP/HTTPS exporter URLs: [`inference_service_validation.go`](https://github.com/kserve/kserve/blob/d7ef6326bd181cfa8cbdafda5dadff49050ae673/pkg/apis/serving/v1beta1/inference_service_validation.go#L120-L130), [`inference_service_validation.go`](https://github.com/kserve/kserve/blob/d7ef6326bd181cfa8cbdafda5dadff49050ae673/pkg/apis/serving/v1beta1/inference_service_validation.go#L188-L220).
- The generated CRD contains `spec.tracing` and its four fields: [`serving.kserve.io_inferenceservices.yaml`](https://github.com/kserve/kserve/blob/d7ef6326bd181cfa8cbdafda5dadff49050ae673/config/crd/full/serving.kserve.io_inferenceservices.yaml#L35499-L35509).

This is sufficient for an ISVC to carry tracing configuration, but it does not by itself create spans or prove any Jira scenario.

### Controller pod configuration — present in the combined tree, but from other PRs

- The shared utility builds `OTEL_RESOURCE_ATTRIBUTES`, including the optional canary attribute: [`utils/tracing.go`](https://github.com/kserve/kserve/blob/fbbd5b49cd9e01bf749a9d44f517de02e056fedf/pkg/controller/v1beta1/inferenceservice/utils/tracing.go#L32-L40).
- It injects service name, exporter endpoint, exporter, sampler, sampler argument, and resource attributes: [`utils/tracing.go`](https://github.com/kserve/kserve/blob/fbbd5b49cd9e01bf749a9d44f517de02e056fedf/pkg/controller/v1beta1/inferenceservice/utils/tracing.go#L68-L83).
- For vLLM it adds `--otlp-traces-endpoint` and `--collect-detailed-traces all`: [`utils/tracing.go`](https://github.com/kserve/kserve/blob/fbbd5b49cd9e01bf749a9d44f517de02e056fedf/pkg/controller/v1beta1/inferenceservice/utils/tracing.go#L86-L98).
- Predictor and transformer reconciliation call the injector; the predictor call supplies the predictor name as the variant: [`predictor.go`](https://github.com/kserve/kserve/blob/fbbd5b49cd9e01bf749a9d44f517de02e056fedf/pkg/controller/v1beta1/inferenceservice/components/predictor.go#L159-L168), [`transformer.go`](https://github.com/kserve/kserve/blob/fbbd5b49cd9e01bf749a9d44f517de02e056fedf/pkg/controller/v1beta1/inferenceservice/components/transformer.go#L159-L168).
- Canary reconciliation builds a canary ISVC with the canary predictor name before building its pod resources: [`predictor.go`](https://github.com/kserve/kserve/blob/fbbd5b49cd9e01bf749a9d44f517de02e056fedf/pkg/controller/v1beta1/inferenceservice/components/predictor.go#L964-L977).
- Existing controller tests prove pod-template behavior for the nil control, vLLM arguments, and the canary resource attribute: [`tracing_test.go`](https://github.com/kserve/kserve/blob/fbbd5b49cd9e01bf749a9d44f517de02e056fedf/pkg/controller/v1beta1/inferenceservice/tracing_test.go#L288-L341), [`tracing_test.go`](https://github.com/kserve/kserve/blob/fbbd5b49cd9e01bf749a9d44f517de02e056fedf/pkg/controller/v1beta1/inferenceservice/tracing_test.go#L344-L444), [`tracing_test.go`](https://github.com/kserve/kserve/blob/fbbd5b49cd9e01bf749a9d44f517de02e056fedf/pkg/controller/v1beta1/inferenceservice/tracing_test.go#L447-L540).

These checks validate environment/argument injection only. They do not send inference traffic or inspect Jaeger spans.

### PR #6198: Python model-server instrumentation — implemented, with gaps

- The PR adds FastAPI and gRPC OpenTelemetry dependencies: [`pyproject.toml`](https://github.com/kserve/kserve/blob/fbbd5b49cd9e01bf749a9d44f517de02e056fedf/python/kserve/pyproject.toml#L45-L51).
- REST instrumentation is installed for inference routes when a tracer provider is returned: [`rest/server.py`](https://github.com/kserve/kserve/blob/fbbd5b49cd9e01bf749a9d44f517de02e056fedf/python/kserve/kserve/protocol/rest/server.py#L150-L163).
- gRPC instrumentation is installed for non-health methods: [`grpc/server.py`](https://github.com/kserve/kserve/blob/fbbd5b49cd9e01bf749a9d44f517de02e056fedf/python/kserve/kserve/protocol/grpc/server.py#L48-L71).
- The response middleware can emit a W3C `traceparent`/`tracestate` response header: [`middleware.py`](https://github.com/kserve/kserve/blob/fbbd5b49cd9e01bf749a9d44f517de02e056fedf/python/kserve/kserve/protocol/rest/middleware.py#L40-L79).
- Exporter selection honors OTLP/console/none and standard OTEL environment variables: [`tracing.py`](https://github.com/kserve/kserve/blob/fbbd5b49cd9e01bf749a9d44f517de02e056fedf/python/kserve/kserve/protocol/tracing.py#L60-L114).

However, `get_tracer_provider()` still creates and returns a `TracerProvider` when no exporter is configured; the REST and gRPC servers therefore still install instrumentation. The local server test explicitly expects a `traceparent` response header on an ordinary inference request: [`tracing.py`](https://github.com/kserve/kserve/blob/fbbd5b49cd9e01bf749a9d44f517de02e056fedf/python/kserve/kserve/protocol/tracing.py#L121-L163), [`test_server.py`](https://github.com/kserve/kserve/blob/fbbd5b49cd9e01bf749a9d44f517de02e056fedf/python/kserve/test/test_server.py#L455-L463). That is inconsistent with the Jira acceptance requirement for zero tracing overhead when `spec.tracing` is omitted.

## Jira case-by-case result

| Jira case | Code evidence | Result | Why it is not fully covered |
| --- | --- | --- | --- |
| 1. Transformer + vLLM, 100% sampler, one connected trace | API config exists; controller injects transformer OTEL variables and vLLM OTLP flags; Python REST/gRPC servers create server spans. | Partial | The requested end-to-end path is not tested. The standard Python transformer outbound path forwards only `x-request-id`, `x-b3-traceid`, and `authorization`, not `traceparent`/`tracestate`, and the PR adds no HTTP/gRPC client instrumentation for that hop: [`model.py`](https://github.com/kserve/kserve/blob/fbbd5b49cd9e01bf749a9d44f517de02e056fedf/python/kserve/kserve/model.py#L44-L66), [`model.py`](https://github.com/kserve/kserve/blob/fbbd5b49cd9e01bf749a9d44f517de02e056fedf/python/kserve/kserve/model.py#L393-L427). |
| 2. Canary request and `isvc.predictor.variant` | Controller resource construction includes the canary variant attribute and has pod-template tests: [`utils/tracing.go`](https://github.com/kserve/kserve/blob/fbbd5b49cd9e01bf749a9d44f517de02e056fedf/pkg/controller/v1beta1/inferenceservice/utils/tracing.go#L32-L40), [`tracing_test.go`](https://github.com/kserve/kserve/blob/fbbd5b49cd9e01bf749a9d44f517de02e056fedf/pkg/controller/v1beta1/inferenceservice/tracing_test.go#L447-L540). | Implementation present; Jira e2e missing | No test sends traffic to the canary or queries a trace backend for a resource attribute. |
| 3. Omitted `spec.tracing` control | Controller injector returns without mutation for a nil spec, and the controller test asserts the OTEL variables are absent: [`utils/tracing.go`](https://github.com/kserve/kserve/blob/fbbd5b49cd9e01bf749a9d44f517de02e056fedf/pkg/controller/v1beta1/inferenceservice/utils/tracing.go#L115-L148), [`tracing_test.go`](https://github.com/kserve/kserve/blob/fbbd5b49cd9e01bf749a9d44f517de02e056fedf/pkg/controller/v1beta1/inferenceservice/tracing_test.go#L288-L341). | Partial | Python server instrumentation is still initialized without an exporter, so the source does not establish zero spans/zero overhead. There is no e2e control test checking a backend for zero inference spans. |
| 4. Inbound `traceparent` parent propagation | FastAPI/gRPC server instrumentors are present, and response middleware exposes the current context. | Partial / unproven | No Jira e2e test exists. The KServe transformer-to-predictor forwarding path does not preserve W3C trace headers and has no client-side instrumentation in this PR, so “all downstream spans are children” is not proven for the multi-component path. |

## Test and infrastructure check

- `test/e2e/isvc/test_tracing.py` is absent from both PR heads and from the fetched current master snapshot.
- PR #6198 adds only [`test/e2e/predictor/test_tracing.py`](https://github.com/kserve/kserve/blob/fbbd5b49cd9e01bf749a9d44f517de02e056fedf/test/e2e/predictor/test_tracing.py). Its first test is skipped pending a Knative field-ref setting, and both tests use a single sklearn predictor rather than transformer + vLLM or a canary: [`test_tracing.py`](https://github.com/kserve/kserve/blob/fbbd5b49cd9e01bf749a9d44f517de02e056fedf/test/e2e/predictor/test_tracing.py#L39-L99), [`test_tracing.py`](https://github.com/kserve/kserve/blob/fbbd5b49cd9e01bf749a9d44f517de02e056fedf/test/e2e/predictor/test_tracing.py#L102-L176).
- That test uses the console exporter and checks model output/log substrings and `service.name`; it does not query Jaeger, compare span trace IDs, assert `isvc.predictor.variant`, exercise the disabled control, or send a caller-supplied `traceparent`.
- The test contains literal placeholder assertions rather than f-strings at lines 96 and 173 (`"{service_name}"` and `"{model_name}"`), so those assertions do not match the actual service/model names.
- `hack/setup/infra/manage.jaeger-helm.sh` exists in the repository, but neither PR changes it and the added predictor test does not invoke it.

## Conclusion

PR #6108 and PR #6198 implement important prerequisites: the v1beta1 tracing API, generated schema/SDK support, Python REST/gRPC instrumentation, and related unit/envtest coverage. They do **not** implement all of the actual Jira e2e work or prove all four Jira cases.

The highest-priority follow-up before marking RHOAIENG-89172 complete is to add `test/e2e/isvc/test_tracing.py` with Jaeger-backed assertions for shared trace IDs, canary resource attributes, the disabled zero-overhead control, and inbound-parent lineage. The transformer outbound path also needs explicit W3C context propagation (or verified client instrumentation), and the Python server’s no-`spec.tracing` behavior needs to be gated if zero tracing overhead is a hard requirement.
