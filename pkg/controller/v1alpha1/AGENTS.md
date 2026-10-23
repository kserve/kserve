# LocalModelCache, node agent and KernelCache controllers

Read the root `AGENTS.md` first. The cache controllers (`localmodel`, `localmodelnode`, `kernelcache`, `kernelcachenode`) run model and kernel caches through Jobs in a shared job namespace. Most review asks are about caches that already exist and about tenants who cannot see that namespace. `trainedmodel` and `inferencegraph` have no area-specific asks yet; the root guide applies.

## Jobs and hashes

A cache is a Job keyed by a hash of its spec, running in a shared job namespace the tenant cannot see. A hash that changes for reasons other than the spec re-imports every existing cache on upgrade; a Job that is never replaced leaves a cache stuck with no path the tenant can take.

- Key an import or download Job by a hash of its spec, the way the shared-PVC import does.
- Replace the Job when the hash drifts and the Job has not completed. Why: "delete the Job and it retries" is not a recovery path. The Job lives in `localModel.jobNamespace`, where a namespace tenant has no RBAC.
- Do not copy the node-agent download path for this. It still launches only when no Job exists or the last one succeeded. A fix there is a follow-up.
- Add `omitempty` to every field you add to a hashed struct, and pin the empty-spec digest in a test. Why: without it every existing cache re-hashes on upgrade. `"imagePullSecrets":null` did exactly that.
- Normalise nil and empty slices the same way before hashing.
- Validate credentials before you build a Job, with the shared Docker-config check. Why: a Job launched without them fails at pull time with a misleading reason.
- In a per-model loop, set `ModelDownloadError` for the failed model and `continue`. Why: the status write at the end must still run.
- Write status fields that other code depends on, such as consumer references, before any loop that can return early.
- Check a named Secret with the API reader before you mount it. Surface a reason the user can act on. Why: a missing or wrongly namespaced Secret leaves the Job pod in `ContainerCreating` and the cache in `ModelDownloadPending` with no message.

In the tree: `localmodel/reconcilers/sharedpvc.go:importSpecHash`, `pkg/credentials/oci_docker_config.go:FetchAndValidateDockerConfigJSONSecret`; `localmodelnode/controller.go:downloadModels` is the path not to copy.

## Trust boundary of the job namespace

Where a cache's Job runs, and where its ServiceAccount and Secrets resolve, depends on the mode. A nodeGroups cache (`LocalModelCache`, and `LocalModelNamespaceCache` without `pvcRef`) uses the shared `localModel.jobNamespace`, so a tenant who can create a namespace cache can name any Secret an admin put there. A `pvcRef` cache runs in the cache's own namespace and resolves credentials there. A ConfigMap knob reused for a new path changes behaviour for every cluster that already set it.

- State in godoc, per mode, where a named Secret or ServiceAccount must live, and that there is no ownership check beyond the name. Why: for a nodeGroups cache it is the shared `localModel.jobNamespace`; for a `pvcRef` cache it is the cache's namespace. Copy the `ServiceAccountName` godoc in `local_model_namespace_cache_types.go`.
- Keep the per-mode namespaces apart in code: a `pvcRef` import Job, its ServiceAccount and its Secrets stay in the cache namespace. Why: moving tenant credentials into the shared namespace lets other caches name them.
- Put tenant-isolation guidance, such as "prefer `pvcRef`", in the sample README and the release note. Not in godoc.
- Do not reuse an existing `inferenceservice-config` knob for a new path. Why: it changes behaviour for every cluster that already set it. `ociInsecureRegistry` shipped for `oci+fetch://` on InferenceService and would have flipped every cache Job. Restrict the flag to the inputs it shipped for, or add a `localModel` knob. Document the knob in `pkg/types/config.go`, the README and the release note.
- Make sure a CA bundle you mount into a Job exists in the job namespace. Why: only the InferenceService reconciler mirrors `global-ca-bundle` into user namespaces. Mirror it, or mark the volume optional.
- Do not hand-create the CA bundle ConfigMap in the e2e. Why: it hides the gap.

## Watches, reads and gates

Neither cache controller scopes its informers, so a cached Get of a Secret starts a cluster-wide informer, and an informer on a missing CRD fails the whole shared manager. A predicate that compares one of two labels misses the other's changes.

- Compare every label the reconciler keys on in a predicate. LocalModel objects carry both `LocalModelLabel` and `LocalModelNamespaceLabel`. A filter on one misses changes to the other.
- Never issue a live Get from an `EnqueueRequestsFromMapFunc` handler.
- Scope every `List` with `client.InNamespace` or `client.MatchingFields`.
- Read Secrets through `client.Reader` in helpers under `pkg/`; the binaries pass the manager's API reader. Why: neither the localmodel controller nor the node agent scopes its cache, so a cached Secret Get starts a cluster-wide informer.
- Honour a feature `enabled` flag in every reconciler of that feature, with an early return. State what disabling does to DaemonSets and CRs already created.
- Ship the CRDs of a controller added to the shared `cmd/localmodel` manager in the default component, or gate its `SetupWithManager` on the flag. Why: an informer on a missing CRD fails the whole manager.
- Use `constants.KServeNamespace` and the label constants in `pkg/constants`.
- Give each component's ServiceAccount and RoleBinding a component-specific name.
- Add a `+kubebuilder:rbac` marker for every group, resource and verb the reconciler issues. Include `delete` for anything you clean up by hand. Why: owner references cannot cross namespaces.

In the tree: the label predicate in `localmodel/reconcilers/localmodelcache_reconciler.go`, the `enabled` early return in `kernelcachenode/controller.go`, `mgr.GetAPIReader()` in `cmd/localmodel/main.go`.

## Status and admission

Status is all a tenant sees of a cache that runs in a namespace they cannot inspect, and the delete-protection validators depend on it being current. Status recomputed from a transiently empty set, or a validator that treats a missing consumer as present, blocks or misleads the tenant.

- Keep the last known status entries when a dependency is transiently unavailable, such as a NotReady node or an empty list. Do not recompute from an empty set.
- Table-test each branch of an aggregate condition with the boundary counts: none ready, all preparing, mixed.
- Treat `IsNotFound` on a tracked consumer as "not a consumer" in the delete-protection validators of both cache kinds. Why: otherwise deletion blocks forever. Document that they rely on `status.*InferenceServices` being current.
- Mirror LocalModel's design in a sibling controller: PV and PVC specs come from the CRD (`PersistentVolumeSpec`), never from a hardcoded `hostPath`. Share the config section, the RBAC list and the base image.
- Factor near-identical helpers across `kernelcache` and `kernelcachenode`. Do not copy them.
- Build names and label values from user input through the truncate-and-hash helper the import Job name uses. Why: the value must fit 63 characters and must not collide with an orphaned cache.

In the tree: `pkg/webhook/admission/localmodelcache/localmodelcache_validator.go:ValidateDelete` and its namespace-cache twin, `localmodel/reconcilers/sharedpvc.go:importJobName`.

## Tests and e2e

A unit test of a helper stays green when the reconciler never calls it, and the modelcache e2e runs against whatever job namespace the ConfigMap names.

- Prove reconcile behaviour with an envtest case through the real reconciler: a Job replaced on hash drift, a status still written after a credential error, a flag flipped to disabled. Why: a unit test of the helper alone stays green when the wiring is wrong.
- In the modelcache e2e under `test/e2e/modelcache/`, read `jobNamespace` from the ConfigMap. Do not hardcode it.
- Mark a test that flips an `inferenceservice-config` key as serial.
- Fail fast on `ModelDownloadError` and include the download Job's pod logs in the message.
- Assert `returncode == 0` on every shell command the test runs.

## Before you open the PR

Have a fresh pass, another agent if you can, try to refute each statement against the diff:

- Every field added to a hashed spec has `omitempty`, and the empty digest is pinned. Existing caches keep their hash.
- A Job is replaced when its spec hash drifts. Credentials are validated before the Job is built, and failures reach status.
- Secrets are read through the API reader. Predicates compare both LocalModel labels.
- No existing ConfigMap knob gained a new consumer without a release note or a separate key.
- Reconcile behaviour has an envtest case through the real reconciler.
