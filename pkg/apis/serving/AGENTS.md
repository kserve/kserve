# API types, conversion and admission

Read the root `AGENTS.md` first. `v1alpha1` and `v1alpha2` of `LLMInferenceService` are both served. `v1beta1` is the `InferenceService` family.

## Served fields are a contract

Objects already stored in etcd decode against the schema the server has now. A field removed or renamed stops them decoding, and a condition reason or status shape is read by clients and downstream controllers from the day it merges.

- Never remove or rename a served field. Why: objects already in etcd stop decoding.
- Retire a field in four steps: keep it in both versions and the generated CRDs, reject new use in the validator, report existing use through a non-blocking condition, remove it in the next API version.
- Check the `release-*` branches and the downstream `rhoai-*` branches before you call a field unreleased. A field on a cut branch is shipped.
- Ship a new CRD minimal. Why: a field without a consumer in this PR is cheaper to add later than to remove.
- Decide nesting and immutability (`self == oldSelf`) before the first release. Why: tightening later is breaking.
- Treat condition reasons and the status shape as API the day they merge.
- Share reason strings as constants between `*_lifecycle.go` and the reconciler.
- Define condition sets with `apis.NewLivingConditionSet` in `llm_inference_service_lifecycle.go` and use the typed `Mark*` helpers.

## Every v1alpha2 field needs its v1alpha1 twin

Both `LLMInferenceService` versions are served and v1alpha2 is the hub. A field present in one version only is dropped in silence when a v1alpha1 client reads or writes the object, and a validation present in one version only lets the other version store what the first rejects.

- Add the twin field to v1alpha1.
- Add both `convert*` directions in `v1alpha1/llm_inference_service_conversion.go`.
- Add a round-trip case to the conversion test. Why: a round trip through v1alpha1 that drops the field in silence is a bug reviewers test for.
- Validate the same rule in v1alpha1 through the v1alpha2 validator: convert the sub-spec and call the exported v1alpha2 function, as `v1alpha1/llm_inference_service_validation.go:validateScaling` does with `convertWorkloadSpecToV1Alpha2`. Do not copy the function. Why: two copies drift, and reviewers reject the duplication.
- When v1alpha1 cannot represent a value, say so in the conversion and choose the lossy path explicitly.
- Use `DeepCopyInto` for ObjectMeta. Set required v1 fields, such as `FailureMode`, to their defaults when the source is nil.
- Do not copy an upstream type into a kserve-owned struct to change a limit. Why: a Go type change breaks every importer, midstream and odh-model-controller included, even when the wire format is identical, and it forces a lossy conversion.
- Patch the generated schema instead, in the `yq` step of `Makefile.generate.mk`, on every API version. Keep `*gwapiv1.HTTPRouteSpec` and similar types as they are.
- List every version the CRD serves in the webhook manifests under `config/webhook/` and in the `+kubebuilder:webhook` markers.

## Shape

The API is read by presets, `baseRefs` and user specs merged together, and by two workload families. A shape chosen for one caller, one mode or one runtime becomes a special case everywhere else, and nesting or immutability cannot be tightened after release.

- Put everything that describes the model under `spec.model`: LoRA, speculator, trust settings.
- Put a field that applies in one mode under a struct named for that mode, next to the discriminator.
- Mirror spec naming in status sub-structs, for example `status.router.scheduler`.
- Avoid repetition such as `speculativeDecoding.speculator`.
- Use `corev1`, `resource.Quantity`, `gwapiv1` and `UntypedObjectReference` before you define a subset struct.
- Expose workload knobs only when they are portable across Deployment and LeaderWorkerSet. `RolloutStrategy` exposes maxSurge and maxUnavailable for that reason.
- Pass a runtime-owned schema through as a `config` map or `RawExtension`. Examples: vLLM `--speculative-config`, EPP plugins. Validate the well-known keys in the webhook. Why: a strict enum locks out values the runtime adds later.
- Keep implementation details the controller must coordinate off the API. Examples: mount paths, ports.
- Give a new field the Gateway API or Kubernetes shape it maps to, for example percent or fraction for mirrors. Do not invent a vocabulary.
- Document on every field that `config_merge.go` combines with a preset whether it is strategically merged, replaced or appended, and what wins between preset, `baseRefs` and the service.
- Give a sub-case with different semantics its own field, not a special-cased shape.

## Markers, CEL and godoc say what is enforced

Godoc becomes the OpenAPI schema and the Python SDK documentation, and markers are what the API server enforces. Where the two disagree, users learn the truth from a rejected apply or a silent acceptance.

- Express required-ness with `MinItems` and without `omitempty`.
- Set list bounds equal to what the controller consumes. Use `MaxItems=1` when only the first entry is read.
- Check string presence with `!= ''` or `MinLength=1`. Why: `has()` is true for `""`.
- Use `self == oldSelf` for a field the controller cannot transition.
- Add the negative CEL test next to the positive one.
- Do not claim "immutable" or "feature gated" in godoc when nothing enforces it on the CRD.
- Write godoc as the user contract: accepted values, defaults, what is ignored and why. Do not describe how the controller implements it. Why: godoc becomes OpenAPI and the Python SDK docs.
- Read the regenerated `openapi_generated.go` diff.
- Put CEL on the CR a human writes: `LLMInferenceService`, `LocalModelCache`. Never on controller-populated mirrors or on status.
- Never relax a schema limit without saying where the equivalent bound now lives: plain `maxItems`, the webhook, or a server dry-run. Prove the CRD still installs on every version. Why: `maxItems` is what the API server prices nested CEL by.
- Drop a CEL rule whose cost forces a `MaxItems` onto a shipped field.
- Treat a tighter `MaxItems` or bound on a served `LLMInferenceService` field as a change to stored services, not only to new ones. Why: llmisvc dry-run creates the rendered spec on every reconcile, so `self == oldSelf` ratcheting does not shield a service that already stores more. Add a compatibility path and a regression test at the previous maximum.

## Admission

A check that lives in the reconciler reaches the user as a condition minutes after `kubectl apply`, and a webhook that calls the API server turns infrastructure errors into user errors. The rendered-spec dry-run means the webhook also validates presets.

- Put a check that reads only the stored spec in `v1alpha2/llm_inference_service_validation.go` or in an `XValidation` rule. Examples: URI scheme, `storageContainerName` with `enabled: false`, negative sizes, annotation values. Why: the user sees it at `kubectl apply`, and the controller's rendered-spec dry-run catches presets too.
- Keep reconciler checks for cluster state only.
- Expose a parser as a method on the type. Why: the webhook and the controller cannot drift.
- Reject an invalid combination. Do not ignore one field in silence.
- Document a name the controller reserves in user space (a container name, a label or annotation key) in the field's godoc, and reject or ignore it in the webhook on services without the feature. Why: a user container with that name otherwise disappears in silence.
- Surface a value the controller must tolerate but cannot act on as a condition or an admission warning.
- In a webhook that calls the API server, turn only `apierrors.IsInvalid` results into `field.Invalid`. Treat timeouts, RBAC and quota as warnings or retryable errors. Why: dry-run creates reserve quota.
- Prefer reconcile-time defaulting, unless another controller needs the result persisted.
- Use the manager's cached client when a webhook does lookups. Justify `failurePolicy: Fail` in a comment.
- Treat `IsNotFound` on a tracked consumer as "not a consumer" in delete-protection validators under `pkg/webhook/admission/localmodelcache`. Document the status-freshness assumption. Never reject the controller's own finalizer or status patches.
- Pass llmisvc-only `inferenceservice-config` keys through `v1beta1/configmap.go:NewIngressConfig` as strings. Parse and bound them in the llmisvc config loader. Why: a typo must not stop `kserve-controller-manager`.

## Generated copies

The CRDs, charts, test bundle, OpenAPI and Python SDK are generated from the Go types, and the generator adds but never deletes. A type change without the regenerated copies fails CI; a removed field leaves orphan models behind.

- Run `make precommit`. It regenerates `config/crd/full`, both Helm CRD charts, `test/crds`, `openapi_generated.go` and the Python SDK. Stage all of them.
- Add a field on a Go type that has a Python model to the keyword-only list in `hack/python-sdk/client-gen.sh`. Why: otherwise the regenerated model shifts every positional argument.
- Run the Python compatibility tests after regeneration.
- Delete orphan models, docs and tests for a removed field by hand. The generator does not.

## Before you open the PR

Have a fresh pass, another agent if you can, try to refute each statement against the diff:

- No served field is removed or renamed. A deprecation keeps the schema and adds validation plus a condition.
- Every new `LLMInferenceService` field has its v1alpha1 twin, both conversions, a round-trip test and the same validation.
- Markers and CEL match what the controller enforces. Godoc reads as the user contract and claims nothing the CRD does not enforce.
- Spec-only checks are in the webhook or CEL, not in the reconciler. Webhook API errors become `field.Invalid` only on `IsInvalid`.
- `make precommit` regenerated the CRDs, both CRD charts, `test/crds`, OpenAPI and the Python SDK. New fields are in the keyword-only list of `hack/python-sdk/client-gen.sh`.
