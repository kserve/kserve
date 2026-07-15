# KServe

KServe is a Kubernetes-native platform for serving machine learning models. The Go codebase implements custom controllers using controller-runtime.

## Constraints

- **Generated files are read-only.** Never hand-edit these - they are overwritten by `make precommit`:
  - `charts/*/README.md`, `charts/*/templates/`, `charts/kserve-crd/templates/`
  - Common Helm helpers (`_utils.tpl`, `_common.tpl`, `_resources.tpl`) synced from `charts/_common/`
  - Quick-install scripts
- **Makefile is the source of truth.** Read `Makefile` and `Makefile.tools.mk` before running or modifying build steps.
- **Distro build tags.** Several files use `//go:build !distro`. Check for `_default.go` / `_distro.go` pairs when adding platform-specific code. `GOTAGS` Makefile variable controls this.
- **`make precommit` before committing.** It handles formatting, linting, code generation, and manifest sync in the right order.

## Project Structure

- API types: `pkg/apis/serving/{v1alpha1, v1alpha2, v1beta1}`
- Controllers: `pkg/controller/{v1alpha1, v1alpha2, v1beta1}`
- Webhooks: `pkg/webhook/admission/`
- CRDs generated from Go types via controller-gen markers

## Commands

Prerequisites: Go (version pinned in `go.mod`) and `make`.

```
make test                       # full Go test suite (downloads envtest assets)
make test-qpext                 # qpext tests (separate Go module under qpext/)
make precommit                  # formatting, linting, codegen, manifest sync
```

For faster iteration on a specific package, run `make setup-envtest` once, then:

```
KUBEBUILDER_ASSETS="$(./bin/setup-envtest use $(go list -m -f '{{ .Version }}' k8s.io/api | awk -F'[v.]' '{printf "1.%d", $3}') -p path)" \
  go test ./pkg/controller/v1beta1/... -run TestSpecificName -v
```

## Testing

- Tests live next to the code they test
- Controller tests mix unit tests (`fake.NewClientBuilder`) and envtest integration suites
- Prefer table-driven tests for pure logic, envtest for controller wiring
- Most controllers use same-package tests. llmisvc uses `_test` packages for integration tests. Follow the convention of whichever controller you're modifying

### envtest

envtest runs a real API server and etcd but NOT built-in controllers (Deployment, ReplicaSet, etc.) or garbage collection. Simulate status updates that external controllers would normally perform.

**Framework and tooling:**
- Suites use Ginkgo/Gomega (`ginkgo.RunSpecs`, `It()`, `Context()`, `BeforeSuite()`)
- Use `pkgtest.NewEnvTest()` from `pkg/testing/` to set up suites - not raw `envtest.Environment{}`. The returned `*pkgtest.Client` wraps the K8s client, environment, and cleanup
- Check `pkg/testing/` for existing Gomega matchers (`BeOwnedBy`, `HaveCondition`, etc.) before writing new ones
- llmisvc has its own `fixture/` package (`pkg/controller/v1alpha2/llmisvc/fixture/`) with builders and envtest setup

**Isolation and cleanup:**
- Each test creates its own namespace
- Clean up resources with `defer` immediately after creation
- Use `retry.RetryOnConflict` when updating resources the controller is also reconciling

**Assertions:**
- Use `Eventually`/`Consistently`, never `time.Sleep`
- Simulate external controller behavior (status updates) with helper functions

## Development Workflow

1. **Analyze** - understand the task, identify affected controllers/types, search for reusable patterns
2. **Test first** - write the test before implementation code
3. **Implement** - minimal, focused changes following existing project patterns
4. **Verify** - run selective tests, then `make precommit` before committing

## Pull Requests

Use the template in `.github/PULL_REQUEST_TEMPLATE.md`. Fill in every section. Focus on **what** changed and **why** - avoid listing implementation details, files changed, or lines of code.

## Controller-Runtime Patterns

### Reconcile Loop

- Reconcile must be **idempotent** - same input run N times produces the same result
- Propagate `context.Context` via function arguments, avoid `context.Background()`
- Handle `NotFound` as success for deleted objects
- Use `Patch` with `MergeFrom` for updates to reduce conflicts
- Return errors for failures (controller-runtime handles backoff). `Requeue: true` only for async work in progress, `RequeueAfter` only for wall-clock delays
- Short-circuit when no-op - already-compliant objects should reconcile with no API calls

### Status and Conditions

- `spec` is user-owned (desired state), `status` is controller-owned (observed state) - never write both in a single API call
- Always include `observedGeneration` in conditions - without it, `Ready: True` may reflect a previous spec generation
- Use conditions (not phase fields) with CamelCase types and positive polarity: `Ready`, `Available`, etc.
- Define condition sets using `apis.NewLivingConditionSet(...)` in `*_lifecycle.go` files. Use typed `Mark*` helpers (`MarkXReady()`, `MarkXNotReady(reason, msg)`) for transitions - never manipulate condition slices directly. See `LLMInferenceService` lifecycle as the reference pattern
- Guard status writes with deep-equal checks - skip when nothing changed to avoid watch churn and infinite reconcile loops
- `reason` is CamelCase and part of the API contract. `lastTransitionTime` updates only when `status` (True/False/Unknown) changes, not on reason/message changes
- For composite conditions, surface the first failing sub-condition's Reason/Message in the parent so users don't have to inspect each one individually
- Use `ClearCondition()` to remove conditions that no longer apply rather than leaving stale values

### Watches and Caching

- Use predicates to drop irrelevant events early
- Prefer `Owns()` and targeted watches over broad `Watches()`
- Add field indexers to avoid expensive list+filter patterns
- Use `APIReader` only when cache staleness causes real correctness issues
- Cached client has no read-your-write consistency - writes hit the API server but are not instantly visible in cache
