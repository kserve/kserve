# InferenceService E2E tests

Tests are organized by component (for example, `predictor/`, `transformer/`,
and `graph/`). Pytest markers select execution groups independently of those
directories. The CI jobs reuse `test/scripts/gh-actions/run-e2e-tests.sh`.

## Standard-mode tracing

The four generic tracing cases are part of the existing `raw` execution group:

- `predictor/test_tracing.py`: canary predictor-variant attributes and omitted
  tracing (no OTEL environment, response headers, or Jaeger spans).
- `transformer/test_tracing.py`: connected transformer/predictor spans and
  preservation of an inbound sampled `traceparent`, including ancestry.
- `common/tracing.py`: shared Jaeger queries, trace assertions, and bounded
  startup diagnostics. Regression tests for these helpers are unmarked and
  do not join the raw execution group.

All four cases carry both `raw` and `tracing`. CI runs them with the other raw
tests using the existing six-worker invocation, and reruns them when the
Gateway API entries enable path-based routing. There is no tracing-only job or
additional pytest invocation. Helm/Kustomize selection follows the existing
raw installation-method matrix.

### Prerequisites and network support

The raw job enables Jaeger through the existing dependency action. Tests query
Jaeger through the Kubernetes service proxy; missing Jaeger is a failure, not
a reason to skip coverage.

The canary case routes 100% of requests to variant `v2` and checks its span
attributes. This requires Gateway API's weighted HTTPRoute backends. CI
explicitly deselects only this case on `istio-ingress`; it runs all four on
`envoy-gatewayapi` and `istio-gatewayapi`, and the other three on ingress.
Tracing uses the same pytest exit status, JUnit reports, and result uploads as
the other raw tests; there is no tracing-specific result checker.

The models use the existing SeaweedFS iris model at
`s3://example-models/sklearn/` and default-service-account S3 credentials.
The custom transformer image runs `custom_transformer.trace_transformer`,
which inherits the production V1 HTTP forwarding path. Shared model seeding
is unchanged, including the OPT-125m download used by other suites.

The tests use the shared `test_namespace` fixture for per-worker namespaces.
Service names are unique per test; transformer fixtures are function-scoped
for xdist isolation. Requests, Jaeger queries, diagnostics, and cleanup use
the fixture's namespace. Setup failures collect service status, related events,
and bounded current/previous container logs before cleanup.

### Focused selection and verification

For an already-provisioned Standard-mode environment, the existing runner
accepts the marker expression `"raw and tracing"`. On ingress, also deselect
`predictor/test_tracing.py::test_canary_trace_contains_predictor_variant`.
Selecting only `tracing` additionally includes existing Knative,
dual-protocol, and LLMISvc tracing coverage.

For this change, local verification is limited to `make precommit`; runtime
verification takes place in the updated PR's CI. Native-vLLM integration and
InferenceGraph router tracing are separate scope.
