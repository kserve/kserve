# LLMInferenceService controller

Read the root `AGENTS.md` first; it defines the terms used here. Each section opens with the problem, then the invariants reviewers hold a PR to, then where the tree already solves it. `GATES.md` beside this file holds the gate table, the slot recipe and the companion-workload rules. Satisfy the invariant; the reference is one shape that does, not the only one.

## How a reconcile runs

`controller.go:reconcile` runs `reconcileBaseRefs`, then `reconcileWorkload`, then `reconcileRouter`, then the scheduler ServiceAccount. `reconcileBaseRefs` merges the presets into `llmSvc.Spec` and dry-runs the rendered spec through the webhook. From then on the merged spec is the only truth about this service; the stored spec is what webhooks and metric collectors see. A decision made on the stored spec, or on a second merge, disagrees with what renders. A check placed in the router runs after the pods have already restarted.

Invariants:

- Every decision reads the merged spec, in or after `config_merge.go:combineBaseRefsConfig`. Nothing merges twice.
- Everything that can reject a service lives in the webhook. The rendered-spec dry-run hits it too.
- A value another component needs, such as the accelerator a webhook or collector reads, is persisted into `Status.Annotations` during reconcile.
- A webhook-defaulted pointer is read nil-safe. Why: fake-client tests and objects arriving through conversion never pass the defaulter that `fixture/envtest.go` registers.
- Status is written in one place. Every component the controller owns has a `Mark<Component>Ready` and `Mark<Component>NotReady` pair. A condition reports the object read back after the write, not `expected`.

In the tree: `telemetry.go:RecordAcceleratorAnnotation`, `router_validation.go` (`ptr.Deref` on `Spec.Model.Name`), `controller.go:updateStatus`, `pkg/apis/serving/v1alpha2/llm_inference_service_lifecycle.go`, `router.go:EvaluateHTTPRouteConditions`, `workload_status.go`.

## Presets are pinned, the controller is not

A service records the preset names it rendered from in `Status.Annotations` and keeps rendering from them after every controller upgrade. Distributions ship version-stamped copies of the presets and rely on that pin: a platform upgrade is not a runtime upgrade, a service stays on its version until its owner moves it, and two services can run two versions side by side. The controller binary carries no such pin. Any output the controller adds on its own lands on every running service at the next controller upgrade, changes its pod template and restarts it. This closed #5949. It is the most frequent review thread in this package, 34 PRs in six months, and the first thing reviewers check.

Invariants:

- A service pinned to an older preset renders byte-identically after a controller upgrade. New pod-template output is something the pinned preset asks for (a slot, a version it declares) or a value carried forward from the child object that already exists.
- A vLLM version check in Go is not a gate. It runs in the controller, so it rewrites every pinned service.
- `config_merge.go:templateFuncs` is a frozen public API: the name, output and escaping of every entry stay; a retired entry moves verbatim, bugs included, to `deprecatedTemplateFuncs`; `missingkey=error` stays.
- Defaults and literal values live in the preset YAML, not in controller code or template functions. Why: a controller default reaches every service pinned to an older preset.
- Every `spec.annotations` key the controller reads is listed in `workload.go:nonPropagatedSpecAnnotations`, so it never reaches the pod template hash.
- A Deployment selector carries only the controller's identity labels, and the stored selector is carried forward. Why: `spec.selector` is immutable; a label that reaches it does not restart the Deployment, it wedges it. An unsatisfiable stored selector is a terminal error, not a retry.
- Label maps are cloned before propagation; Deployment metadata and pod template never alias one map.
- User-set resources, `Args` and `Command` stay as written. An engine flag is emitted only when the spec sets it; vLLM keeps its own defaults; a conflict produces a warning, not an overwrite.
- Pinning adds keys, it never rewrites them: a preset added in release N+1 lands pinned to its new name beside N-era pins on existing services. A new preset is rendered with that mixture in a test.
- Preset lookup prefers the service namespace, so a tenant copy of `kserve-config-llm-*` answers to the well-known name. Preset-only annotations are trusted only when the preset came from `constants.KServeNamespace`.
- A pinned preset name that no longer resolves is terminal with reason `ConfigNotFound`, not retried.
- Upgrade safety has an `*_int_test.go` case: the previous release's child object created by hand, one reconcile, `metadata.generation` unchanged under `Consistently(...).WithTimeout(3 * time.Second)`.
- A controller feature that reads what a preset wrote (a cache directory, a flag, a mount) handles the output of every released preset, frozen legacy template functions included, or scopes itself to presets that declare the new shape. Why: a service on an older preset otherwise gets a companion that scans the wrong directory.
- A deliberate one-time restart is named in the release note.

In the tree: `config_merge.go:WellKnownConfigResolver` (`Attach` pins only keys not yet in status), `config_merge.go:getConfig`, `workload.go:deploymentSelectorLabels`, `workload.go:PreserveDeploymentSelector`.

For the slot recipe with its three shipped shapes, the table of which gate fits which change, and the invariants for companion workloads and shared resources, read `GATES.md` in this directory before you change rendered output.

## Sub-reconciler shape

Each component the controller owns is one `reconcileX(ctx, llmSvc, cfg)` that builds the expected object, writes it through the shared CRUD helpers and reports its condition. What fails review is a component that cannot be stopped or removed, a spec problem that retries forever, a condition reported from the intended object instead of the observed one, cleanup skipped by an early return, and a message the tenant cannot act on.

Invariants:

- "Should delete" is derived from `llmSvc` inside the function, not threaded in as a parameter. Every `reconcileX` has a stop path (`MarkXNotReady("Stopped", ...)` then delete) and a feature-removed path (`MarkXUnset()` then delete). Delete needs only name and namespace and tolerates `NotFound` and a missing CRD.
- Nothing returns before the cleanup of owned objects has run. Why: an early `return nil` orphans them, RBAC and routes included.
- The delete path depends only on the objects the deleting service owns: cleanup lists them instead of reading the current spec or the global ConfigMap, also when another service in the namespace still has a companion. Why: otherwise a missing or malformed `inferenceservice-config` leaves a service that never used the feature stuck in `Terminating`.
- A companion the controller places beside a consumer (an evictor, a permission-fix Job) carries the consumer's tolerations and node selection. Why: on a tainted GPU node the consumer runs and the companion stays Pending, so the service never becomes Ready. `GATES.md` has the rest.
- An object shared between services or between prefill and decode has one stated precedence for conflicting settings, a terminal reason for a conflict that cannot converge, and a sharing key computed from normalized values, so unset and the explicit default compare equal. When one peer changes or drops its reference, through the spec or a `baseRef` config, the others are enqueued.
- Cleared fields are compared explicitly. Why: the `semantic*IsEqual` helpers use `DeepDerivative`, so nil in `expected` means "don't care".
- Stopped and deleting services are skipped in every enumeration: metrics, cleanup.
- A new Deployment, LeaderWorkerSet or Service ships with its readiness condition in the same PR. Only states the LLMInferenceService API exposes drive `Ready`.
- The `BeforeDryRun` and `AfterDryRun` hooks of the shared update are used instead of a second dry-run. Why: the update already dry-runs for defaults.
- A no-op reconcile of an unchanged service issues no API writes; the PR body states the write count for N reconciles on master and on the branch.
- A spec problem that will not fix itself sets the component condition with its own CamelCase reason and does not back off. A plain error means infinite backoff. Terminal and transient paths have distinct reasons. Failures in peer or group bookkeeping stay non-terminal: they must not stop the service's own workload.
- Nothing further up the chain overwrites the component's reason before status is written.
- API-server field errors are returned unwrapped, so the one message the user sees is the real one; reconciler errors are wrapped with the service `namespace/name`, so logs are correlatable. A dry-run rejection of the rendered spec is reported through a message that does not name the `<svc>-validation-xxxxx` object, which the tenant cannot look up.
- Condition messages are written for the tenant who can act: `spec.*` paths and rule names, not internal indexes; both the annotation and the ConfigMap key that lift a limit; lists capped with "and N more"; whether stored objects keep serving.
- A deprecation is reported through a Warning event or a condition keyed to the deprecated input being present, never through an `Info` log on every reconcile.
- Only pod shapes that a released preset or controller produced are stripped or converted, and the comment names the release.

In the tree: `scheduler.go` and `tokenizer.go` as models, `kmeta.ChildName` for names, `lifecycle_crud.go:Reconcile` and `Delete`, `scheduler.go:propagateComponentDeploymentStatus`, `utils.GetForceStopRuntime`, `router.go:ErrPreconditionNotMet` and `reconcile.TerminalError`, `config_merge.go:validationDetail`, the `RoutingPreconditionNotMet` wording in `router.go`.

## Pod spec mutations and the scheduler config

One spec renders into four pod templates (`Spec.Template`, `Spec.Worker`, `Spec.Prefill.Template`, `Spec.Prefill.Worker`) through several builders, and distributions rename images and containers in their presets. A mutation applied per builder, keyed on an image or container name, or derived from a list index, breaks on one of those.

Invariants:

- Container settings are injected or migrated once, post-merge, and the mutation walks all four templates.
- Everything that puts model bytes into a pod goes through one function that every workload shape and the scheduler call.
- New mount destinations sit under the existing `/mnt/<feature>` parent, and the main container's `volumeMounts` are asserted in a test.
- Mount paths and volume names derived from user strings are sanitised and hash-suffixed on collision, never index-based. Why: reordering `spec.model.lora.adapters` must not move a running pod's mounts.
- A spec-derived list is resolved once and passed to every consumer.
- An escape hatch is a spec field or an annotation, not an env var's value. Why: a `valueFrom` is invisible to the controller.
- Nothing branches on an image name or a container name; it branches on a spec field, an explicit parameter, or a marker the preset sets.
- Flag parsing reuses the existing helpers, which handle every flag form on `Command` and `Args`.
- Default EPP plugins are added in one place. Feature removal in the scheduler config is handled explicitly. Why: the running `--config-text` is carried forward across upgrades.
- Every scheduler config option iterates all `schedulingProfiles`. Why: prefill/decode has `prefill` and `decode` profiles and no `default`. A `pluginRef` resolves by its `name` alias as well as by `type`.
- A version-dependent EPP transform gates on the expected scheduler template version, not on the current Deployment.
- Data-plane ports are computed in one place from the InferencePool target ports and the ports on the pod; no `8000` literals.
- A half-set `rolloutStrategy` is completed from the target family: Deployment 25%/25%, LeaderWorkerSet `maxUnavailable: 1` and `maxSurge: 0`.
- DisaggregatedSet readiness is compared against updated replicas. Why: `ReadyReplicas` counts old and new pods.

In the tree: `combineBaseRefsConfig` post-merge hooks, `workload_storage.go:attachModelArtifacts`, `workload_lora.go:sanitizeLoRAPathSegment`, `loraMountSegments`, `enumerateLoRAAdapters`, `userSuppliedLoRAConfig` with the `hasValueFromLoRAConfig` warning, `scheduler_plugins.go:hasPluginType`, `scheduler.go:filterArgs`, `scheduler.go:schedulerConfigText`, `scheduler.go:preserveSchedulerConfig`, `mutateSchedulerConfig`.

## Routing

Presets resolve from the service namespace and from the kserve namespace, so a bare name is ambiguous. One spec renders into several routes and InferencePools. Cross-route precedence is gateway behaviour and differs between Istio and Envoy Gateway. Downstream controllers follow `status.router`.

Invariants:

- Every cross-service lookup, index and dedupe is keyed by `namespace/name`. An empty `ref.Namespace` means the route's namespace.
- Owned HTTPRoutes and group members are found by expected name plus `metav1.IsControlledBy`. Why: labels can be spoofed.
- Deleting and stopped members are filtered once, and the same list feeds the validation pass and the backendRef pass.
- Every member field that group code reads is in the change-detection set. Why: otherwise siblings keep a stale verdict until an unrelated event wakes them.
- When one spec renders into several objects: the whole set is dry-run first, writes go add-before-remove so every served match stays stored at each step, stale objects are pruned last, and the failing object is reported while the rest keep serving. The mixed-move shape has a test.
- `status.router.gateways[].httpRoutes` stays complete and `rules[].name` is never rewritten. Why: AuthPolicy and odh-model-controller follow them. The downstream follow-up is named in the PR.
- A new match type, rule order or shard is verified on Istio and on Envoy Gateway with a contrast expected to flip. Istio ranks Exact over Prefix over Regex and ties on its generated route name; Envoy Gateway renders a method as a header. Each gateway assumption is documented next to the key function and pinned with a test.
- User-controlled input is bounded before any pairwise pass; annotation-sourced limits are clamped to the constant the cleanup loop enumerates. Why: the webhook is not the only writer.
- When a function that took one object takes N, every caller reads the whole set: URL discovery, status, InferencePool migration, group cleanup.
- The HTTPRoute budget is counted after LoRA expansion: 16 rules, 64 matches per rule, 128 per route. Runtime-specific rules are stripped when unused.

In the tree: `router_group.go` and `llmisvcconfig_controller.go` for keyed lookups, `router_group.go:finalizeGroupMembership`, `router_group_handler.go:trafficFieldsChanged`, `config_merge.go:stripModalityRouteRules`.

## Config, gates and watches

The controller pod also serves the validating and conversion webhooks with `failurePolicy: Fail`, so a restart makes creates, updates and v1alpha1 reads fail for seconds. A bad `inferenceservice-config` value parsed in the shared loader stops `kserve-controller-manager`, not only llmisvc. A new input read in reconcile without a watch goes stale until an unrelated event.

Invariants:

- llmisvc-only `inferenceservice-config` keys pass through the shared ingress config as strings and are parsed, defaulted and bounded in the llmisvc loader, so a bad value degrades only llmisvc. Every chart-rendered value has a `| default` in `charts/_common/common-patches/configmap-patch.yaml`.
- A gate lives where the table says: controller-wide behaviour on `llmisvc.Config`, per-service opt-in on an annotation paired with the ConfigMap key, pod-template output on a slot, startup facts in reconciler fields.
- The cache options in `cmd/llmisvc/main.go` are a contract: Secrets and ConfigMaps are cluster-wide with the llmisvc label selector so `Owns()` works in workload namespaces, and a namespaced `ByObject` entry does not merge with that.
- Every new input read in reconcile has a `Watches` or `Owns` with a mapper that diffs only the keys consumed and enqueues only the consumers of that object, following references from `baseRefs` and `prefill` too. Why: enqueueing every service in the namespace is the shape reviewers send back.
- Admission-time work or a new webhook path is named in the release note.
- A metrics collector runs only on the elected replica, skips stopped and deleting services, classifies unknown accelerators as `unknown`, and never restates a metric the EPP or vLLM already exposes.

In the tree: `v1beta1.NewIngressConfig`, `config_loader.go:NewConfig`, `mgr.Elected()`.

## Tests

The fake client has no webhook, so a validator or defaulter never runs there. The defaulter is registered only in envtest. A same-package test of a helper stays green when the wiring around it is wrong.

Invariants:

- A controller change is proven by a `controller_*_int_test.go` case in `package llmisvc_test` through the real `LLMISVCReconciler`: create the service, change the input, assert the child objects. Internals are reached through an `*_export_test.go` shim. Same-package tests cover pure functions with edge-case tables only. `lifecycle_crud.Reconcile` is not called directly from envtest.
- The namespace comes from `fixture/test_namespace.go:NewTestNamespace`, which names it after the spec, creates the default ServiceAccount envtest lacks and registers `DeferCleanup`. Older specs with `kmeta.ChildName(name, "-test")` plus `defer` are not the model.
- Objects come from `fixture/llmisvc_builders.go` and `fixture/gwapi_builders.go`; well-known presets from `fixture/required_resources.go:SharedConfigPresets`; preset rendering is covered in `config_presets_test.go`.
- Every URI scheme that a change to model-artifact attachment touches has a `controller_workload_storage_int_test.go` case.
- Migration fixtures come from real previous-version output, not from current output with one field flipped.
- An upgrade proof asserts only on objects and conditions both controller versions have, replays the previous controller's child object from a fixture the test captured itself (a capture mode behind an env var), and runs on the base commit and on the branch, so the reviewer sees it pass on one and fail on the other. Why: a test that uses the new condition type cannot run on the base, and a green run on the branch alone says nothing about existing services.
- Pull-secret merging is tested on a cluster. Why: envtest has no ServiceAccount admission plugin.
- Every cleanup path has a fake-client case: stop, refs removed, group prune. Why: the webhook is absent there, and that is where the bugs hide.

## Before you open the PR

Have a fresh pass, another agent if you can, try to refute each statement against the diff:

- Everything that existed before keeps working: stored services decode and reconcile, carried-forward configs load on the new operand, pinned presets render. This is separate from, and comes before, the restart question.
- Nothing added to a pod spec reaches a service pinned to an older preset. An `*_int_test.go` asserts the Deployment generation is unchanged across reconciles.
- The PR lists the stored shapes the change must accept: each released preset, carried-forward child content, the output of earlier migrations. A shape the new code rejects has a gated migration and a test from a real previous-version fixture.
- Every decision reads the merged spec after `combineBaseRefsConfig`. Spec-only validation lives in the webhook.
- Every new `reconcileX` has a stop path, a feature-off delete path, and its own `Mark<X>Ready` and `Mark<X>NotReady` pair.
- A user-fixable spec problem sets a condition and does not back off. Nothing returns nil before owned objects are cleaned up.
- Owned objects are found by name plus `IsControlledBy`. Lookups are keyed by `namespace/name`.
- The primary test is in `package llmisvc_test`, runs through the real reconciler with the `fixture` builders, and fails when the change is reverted.

For an operand bump (EPP, router sidecar, vLLM, tokenizer image), also refute these:

- Every config field, plugin and flag the new version rejects is listed from the type diff between the tags, and each one has a migration gated on the new version, proven on a real previous-version config. `make check-image-flag-drift` passes locally.
- The tests for the previous version stay and the new version's cases are added beside them. A test rewritten from N to N+1 is a deleted test.
- Metric renames are applied to the dashboards under `docs/samples/llmisvc/e2e-gpt-oss/grafana/` and named in the release note.
- The body states what keeps working and what restarts as two separate statements, and names the version-stamped presets that are unaffected.
