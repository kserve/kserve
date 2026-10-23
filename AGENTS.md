# KServe

KServe serves machine learning models on Kubernetes. The Go code implements controllers with controller-runtime.

## Terms

The guides use one word for each of these.

- service: an `LLMInferenceService` or `InferenceService` object.
- child object: an object the controller writes for a service. Examples: Deployment, LeaderWorkerSet, HTTPRoute, Service, Job.
- preset: an `LLMInferenceServiceConfig` under `config/llmisvcconfig/`.
- pinned: a service renders from the preset names stored in its status, not from the newest presets.
- render: build the child objects from the merged spec.
- restart: a pod template change that replaces running pods.
- gate: a check that keeps new output away from services that did not opt in.
- slot: a key a preset declares for the controller to fill.
- operand: a container image the controller runs for a service. Examples: vLLM, EPP, router sidecar, tokenizer.
- compatible: an object, config or request that worked on the previous version still works on this one, unchanged. This is correctness; nothing waives it.
- disruption: a restart, a roll, a re-schedule. This is an operational cost; it is gated when avoidable and named when deliberate.
- companion: a child object that runs its own pods for another workload. Examples: the EPP scheduler, the tokenizer, the routing sidecar, the storage initializer, a KV cache evictor, a permission-fix Job.

## Area guides

Read the guide for a directory before you change files in it. Each guide lists the checks reviewers make in that area and the code to copy. Harnesses that load skills get the same map from the `kserve` skill under `.agents/skills/`.

When you add or change a guide line, say for which kinds, modes and build paths it holds, and check that scope against the tree. Why: a line distilled from one review is true for the case that review covered; applied wider, it sends an agent to a wrong manifest with no hesitation.

| Directory | Guide covers |
|---|---|
| `pkg/controller/v1alpha2/llmisvc/` | preset pinning, sub-reconciler shape, status, routing, watches, tests; `GATES.md` there for gates, slots and companion workloads |
| `config/llmisvcconfig/` | preset variants, gated flags, image pins, HTTPRoute budget |
| `pkg/apis/serving/` | API types, v1alpha1 and v1alpha2 conversion, CEL, webhooks, godoc |
| `pkg/controller/v1alpha1/` | LocalModelCache, node agent, KernelCache: Jobs, hashes, job-namespace trust boundary |
| `test/e2e/` | pytest harness, isolation, markers, what an e2e proves |
| `docs/samples/llmisvc/` | LLMInferenceService samples that match the selected preset and run as pasted |

## Constraints

Most files under `charts/`, `config/crd/` and the SDK are generated, and the Makefile is the only thing that regenerates them in the right order. A hand edit there is overwritten by the next `make precommit`, and a build step run outside the Makefile drifts from CI. Platform code is split by build tag, and `controller-gen` does not honour tags.

- Do not edit generated files. `make precommit` overwrites them. Generated files: `charts/*/README.md`, `charts/*/templates/`, `charts/kserve-crd/templates/`, `charts/kserve-runtime-configs/files/**`, the `serving.kserve.io_*.yaml` schemas under `config/crd/full/` and everything under `config/crd/minimal/`, `test/crds/serving.kserve.io_all_crds.yaml` and `test/crds/gateway-inference-extension.yaml`, `pkg/openapi/openapi_generated.go`, `zz_generated*.go`, the Python SDK models, the common Helm helpers synced from `charts/_common/`, and the quick-install scripts.
- Edit by hand the inputs that sit next to them: the `kustomization.yaml` files and conversion-webhook patches under `config/crd/full/`, and the external CRD fixtures under `test/crds/` (Gateway API, the GIE v1alpha2 pool, LeaderWorkerSet, DisaggregatedSet, KEDA, Knative, Istio, OpenTelemetry), which change on a dependency bump.
- Do not edit released artifacts. `install/vX.Y.Z*/` and shipped chart versions are immutable.
- Read `Makefile` and `Makefile.tools.mk` before you run or change a build step. The Makefile is the source of truth.
- Put local and downstream make targets in `Makefile.overrides.mk`. The `Makefile` includes it when present.
- Keep platform-specific code in `_default.go` files with `//go:build !distro`. The distro half lives in the midstream repo as `*_odh.go` with the same hook signature. `GOTAGS` selects the build.
- Do not put a `+kubebuilder:rbac` marker in a distro-only file. `controller-gen` ignores build tags, so the marker lands in the upstream `config/rbac/role.yaml`. Put markers for platform-only API groups in a package only the distro build imports, and add that package to the distro's generation roots through the `manifests-distro` hook in `Makefile.overrides.mk`. Why: `controller-gen rbac` reads markers only from the `paths=` it is given, so a package outside the roots generates nothing and the controller fails with Forbidden.
- Run `make precommit` before each commit. It formats, lints, generates code and syncs manifests in the right order.
- Read `git status` after `make precommit` and stage every regenerated file.
- When you remove an API field, delete its Python SDK model and docs by hand. The generator does not delete them.

## Project structure

- API types: `pkg/apis/serving/{v1alpha1, v1alpha2, v1beta1}`
- Controllers: `pkg/controller/{v1alpha1, v1alpha2, v1beta1}`
- Webhooks: `pkg/webhook/admission/`
- CRDs are generated from Go types through controller-gen markers.

## Commands

Prerequisites: Go at the version pinned in `go.mod`, and `make`.

```
make test                       # full Go test suite (downloads envtest assets)
make test-qpext                 # qpext tests (separate Go module under qpext/)
make precommit                  # formatting, linting, codegen, manifest sync
```

To iterate on one package, run `make setup-envtest` once, then:

```
KUBEBUILDER_ASSETS="$(./bin/setup-envtest use $(go list -m -f '{{ .Version }}' k8s.io/api | awk -F'[v.]' '{printf "1.%d", $3}') -p path)" \
  go test ./pkg/controller/v1beta1/... -run TestSpecificName -v
```

## Shipped contracts

Every change to a running system answers two different questions, and the guides keep them apart.

First, does everything that exists keep working: stored CRs decode, carried-forward configs load on the new operand, pinned presets render, API clients and metric consumers see what they saw. That is backward compatibility, a correctness bar with no opt-in and no release note that can waive it. A service that stops reconciling or a scheduler that fails to start after an upgrade is a bug, however well announced.

Second, does the change disrupt what is running: pods restart, a rollout happens, placement changes. That is an operational cost. It is gated when a gate exists, it is deliberate and named in the release note when it does not, and it is never the way a compatibility break is "handled".

Treat each item below as API. Removal or rename needs a deprecation path and a release note. Anything on a `release-*` branch or a downstream `rhoai-*` branch counts as shipped.

- Served CRD fields. Removal breaks decoding of objects already in etcd.
- Condition types, condition reasons, and the `status` shape.
- Controller CLI flags. Keep a removed flag as a deprecated no-op for one release.
- Metric names and label values.
- Preset template functions.
- What an unchanged service renders to. A change that restarts existing pods, changes a `kubectl apply` outcome, a status field or the number of child objects ships opt-in. The opt-in is an `inferenceservice-config` key with a `serving.kserve.io/...` annotation override, and the default keeps today's behaviour. Copy `loraModelRoutingStrategy`.
- Stored state is the input set. Compatibility is judged against what exists in clusters, not against what the repo ships today: CRs in etcd, child objects whose content the controller carries forward (a scheduler's `--config-text`, a selector, a hash), presets pinned by name. Every shape a previous controller version wrote, including the output of its own defaults and migrations, is an input the new version must accept or migrate. Enumerate those shapes from previous releases (`git show <tag>:<path>`, a captured child object), not from the current tree. Why: a field a release removed is not in any shipped preset, but it is in every config the previous controller decomposed, and the strict decoder stops the binary.
- Name the gate in the PR body for every change to rendered objects or behaviour. The llmisvc guide has the table of gates.

## Testing

A test that passes without the change it was written for proves nothing, and a helper tested alone stays green when the wiring around it is wrong. Reviewers ask for the test at the layer where the bug would have shown.

- Keep tests next to the code they test.
- Use table-driven tests for pure logic and envtest for controller wiring.
- Keep expectations inline. Use a golden file only for an oracle produced independently of the code.
- Follow the test package convention of the controller you change. Most controllers use same-package tests. llmisvc uses `_test` packages for integration tests.
- Revert your change once and confirm a new test goes red before you push, or run the new tests on the base commit with `go test -overlay`. A test that stays green without the change proves nothing.
- Give every new error branch a case that fails when the branch is replaced by its happy-path return. Why: swapping `return err` for `return nil` and keeping the suite green is a recurring review ask.

### envtest

envtest runs a real API server and etcd. It does not run built-in controllers, garbage collection, or the ServiceAccount admission plugin. Simulate the status updates that external controllers would make.

- Write suites with Ginkgo and Gomega: `ginkgo.RunSpecs`, `It()`, `Context()`.
- Start envtest in `BeforeSuite`, not `SynchronizedBeforeSuite`. Why: each Ginkgo parallel process owns its API server. See `pkg/controller/v1alpha2/llmisvc/suite_test.go`.
- Set up suites with `pkgtest.NewEnvTest()` from `pkg/testing/`, not a raw `envtest.Environment{}`. The returned `*pkgtest.Client` wraps the client, the environment and cleanup.
- Use the matchers in `pkg/testing/` before you write one: `BeOwnedBy`, `HaveCondition`, `BeSematicEqual`, `HaveContainerImage`, and for HTTPRoutes `HaveGatewayRefs`, `HaveBackendRefs`, `HaveHeaderMatch` in `pkg/testing/httproute_matchers.go`. Reviewers ask you to replace a hand-rolled walk over `Rules[].BackendRefs[]`.
- For llmisvc, use the builders and envtest setup in `pkg/controller/v1alpha2/llmisvc/fixture/`.
- Create one namespace per test and clean up with `DeferCleanup`.
- Use `retry.RetryOnConflict` when you update an object the controller also reconciles.
- Use `Eventually` and `Consistently`. Never use `time.Sleep`.
- Replace a helper with a boolean parameter by two assertions or a Gomega matcher.
- Start a helper that simulates an external controller's status with `if envTest.UsingExistingCluster() { return }` from `pkg/testing/config.go`. Why: the same spec then runs against a real cluster with `USE_EXISTING_CLUSTER=true`.

## Controller-runtime patterns

A reconcile that writes when nothing changed, retries a problem only the user can fix, or reports the object it meant to write instead of the one stored, produces churn, backoff loops and conditions that lie. These are the controller-runtime asks that recur in kserve reviews.

- Make reconcile idempotent. A no-op reconcile issues no API writes.
- Read what the shared helpers already issue before you add a Get, a dry-run or an Update.
- Pass `context.Context` as a function argument. Do not use `context.Background()`.
- Treat `NotFound` as success for a deleted object. The generic delete helpers already tolerate `NotFound` and a missing CRD.
- Return an error only when a retry can fix the problem. controller-runtime adds backoff to every error.
- Set a condition with a CamelCase reason for a spec problem the user must fix, and do not back off.
- Keep a kind name out of the error message of a generic helper. Derive it from the object, or take it as an argument.
- Do not `return nil` before the cleanup of objects you own has run.
- Never write `spec` and `status` in one call. `spec` is user-owned, `status` is controller-owned.
- Write status from one place per controller.
- Report in conditions the object you read back, not the object you were about to write.
- Select a condition message, a first error or a preferred object over sorted keys, never by ranging over a map. Why: map order changes between reconciles, so a message-only difference writes status and requeues.
- Define condition sets with `apis.NewLivingConditionSet(...)` in `*_lifecycle.go` and use the typed `Mark*` helpers.
- Set `observedGeneration` on conditions. Surface the first failing sub-condition in the parent. Use `ClearCondition()` for a condition that no longer applies.
- Emit events for errors and transitions only. Never emit an event for success, for a no-op, or for a steady state seen again on every reconcile.
- A callee either marks the condition or returns the error for the caller to report. Never both.
- Use predicates, `Owns()` and targeted watches. Use a field indexer instead of list-and-filter.
- Add a watch and a mapper for every new input the reconciler reads.
- Find owned objects by deterministic name plus `metav1.IsControlledBy`. Never by labels alone.
- Use `client.Reader` for a one-off read of a kind you do not watch. Why: a cached Get starts a cluster-wide informer.
- Build child names with `kmeta.ChildName`.
- Use `constants.KServeNamespace`, `network.GetClusterDomainName()` and the constants in `pkg/constants`. Do not use literals.
- Add `omitempty` to every field you add to a struct that feeds a hash, and pin the empty-spec digest in a test. Why: without it every existing object re-hashes on upgrade.
- Replace a Job with a deterministic name when its spec hash drifts. Why: tenants cannot delete the Job.
- Add a `+kubebuilder:rbac` marker for every group, resource and verb the reconciler issues. Include `delete` for anything you clean up by hand.
- Reuse `kserve-leader-election-role`.

## Pull requests

Use the template in `.github/PULL_REQUEST_TEMPLATE.md`. Fill in every section.

Before you open the PR:

1. Read the acceptance criteria of the linked issue. State in the body where you deviate.
2. Run `make -j1 precommit`. Confirm it is clean and that `git status` shows every regenerated file staged. Why: tests that do not compile, lint failures and stale generated files take the whole first review round.
3. Have a fresh pass over the diff. Use another agent, or yourself after a break. The pass reads the area guide and tries to refute every claim in the body: no restart on upgrade, the test fails without the fix, behaviour is unchanged for existing services. What that pass finds, the first review round finds. Replaying past reviews with these guides reproduced the maintainers' first-round comments.
4. Run one pass per area guide when the PR spans areas. Do not run one pass over the whole diff.

Rules for the PR:

- Make one PR do one thing. Put drive-by fixes, formatting and tooling changes in their own PR.
- Give one sentence of justification for every deleted resource, comment or test.
- Rebase on master before you push.
- State in the body what changed and why, in plain Kubernetes terms.
- Trace every claim to a run or a source line. Examples of claims: "no behaviour change", "restarts once", "2x faster". Say what you could not verify.
- Write the release note for the operator: what to turn on, what consumers must read, what restarts or needs more disk, what a rollback leaves behind, dependency version floors.
- On re-review: fetch the latest head, push one commit per review thread, run `make -j1 precommit` again, reply on every thread.
- Write a code comment only for an invariant the code cannot show, in one or two lines. Delete a comment that paraphrases the next line.
