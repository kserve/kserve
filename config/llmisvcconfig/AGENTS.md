# LLMInferenceServiceConfig presets

Read the root `AGENTS.md` first. For anything the controller fills in, read `pkg/controller/v1alpha2/llmisvc/AGENTS.md` as well.

## Pinning and restarts

A service renders from the preset names stored in its status; the controller binary is not pinned. Distributions build on this: they ship version-stamped copies of these presets (`v3-2-0-kserve-config-llm-decode-template`) and thin accelerator variants that declare only a name, annotations and an image on top of the base, so a service stays on its version until its owner moves it, and a change to a base template reaches every variant built on it. A preset that a service references cannot be deleted: a finalizer and the `ConfigInUse` condition hold it, and a webhook blocks deletion of the well-known ones.

- On an install with fixed preset names, every edit here re-renders every vLLM Deployment or LeaderWorkerSet, EPP and tokenizer that uses the preset, on the first reconcile after upgrade.
- Gate a change that restarts pods, or name the restart in the release note.
- Make the rendered pod template byte-identical when a new variable is unset. Wrap the flag like the `{{ if .GlobalConfig.EnableTLS }} ... {{- end }}` blocks in `config-llm-template.yaml`. Leave no empty argv entry and no stray ` \` continuation line.
- Add an `{{ else }}` branch only when the binary's default differs from the off state. Examples: `--secure-serving`, `--secure-proxy`.
- Assert the default render is unchanged in `pkg/controller/v1alpha2/llmisvc/config_presets_test.go` and in the `TestReplaceVariables_*` tests. Reviewers ask for a master-vs-branch render diff.
- Declare a slot in every preset variant that opts in, with a one-line comment that the controller fills it. Existing slots: `KSERVE_KV_TRANSFER_ARGS`, `KSERVE_TRACING_CONFIG_VERSION`, `serving.kserve.io/llmisvc-revision: placeholder`, `internal.serving.kserve.io/kv-cache-shm-percent-of-cpu`. The recipe is in the llmisvc controller guide.
- Keep tunables, such as the shm percentage, on the preset. Why: retuning then ships as a new preset.
- Declare a companion the controller creates for a spec field (an evictor, a permission-fix Job) as a container template in the preset, with its image, resources and security context. Why: the controller is not pinned, so a companion default in Go rolls every pod with that field on upgrade; a distribution overrides the image in its preset copy.
- Treat `config_merge.go:templateFuncs` as a frozen contract. Do not rename, remove or change the output of a function.
- Write a value that reads nothing from the spec literally in the YAML, not in a template function.

## What the presets are and how a service is composed

A preset is a partial `LLMInferenceService` spec. The controller does not pick one preset per service; it picks one per role the spec has, strategically merges them in order, lowest precedence first, then merges the user's `baseRefs` and finally the service's own spec on top (`config_merge.go:combineBaseRefsConfig`). Everything a service renders comes out of that merge, so a line missing from one variant is a behaviour difference between two services, or between a leader and its worker, that nothing in the spec explains.

Which presets a service gets is decided by its spec:

| Family | Preset | Attached when |
|---|---|---|
| vLLM workload, non-P/D | `config-llm-template` | no `prefill`, no `worker`; the omni runtime layers `config-llm-omni-template` on top of it, the sglang runtime uses `config-sglang-template` instead |
| vLLM workload, non-P/D, multi-node | `config-llm-worker-data-parallel` | `worker` set and `parallelism` data-parallel |
| vLLM workload, P/D | `config-llm-decode-template` and `config-llm-prefill-template` | `prefill` set, one preset per role; with `worker`, the `-worker-data-parallel` pair instead |
| multi-port data-parallel | `config-llm-{decode,prefill}-worker-data-parallel-multi-port` | only through the user's `baseRefs`; the preset carries `serving.kserve.io/replaces-preset`, so the controller suppresses the default DP preset instead of merging underneath it |
| scheduler | `config-llm-scheduler`, plus one of `config-llm-scheduler-eppconfig-{default,default-pd,modality}` | `router.scheduler` managed; the eppconfig preset is picked by shape (P/D, omni, else default) and only when the pinned router version supports presets |
| tokenizer | `config-llm-tokenizer` | the scheduler needs a tokenizer |
| route | `config-llm-router-route` | `router.route.http` managed; not versioned, because it depends on the Gateway API provider |
| tracing | `config-llm-tracing` | `spec.tracing` set |

Pipeline-parallel names exist in the controller but have no manifest. The selection table in `config_merge.go` is the source of truth; when it and this table disagree, fix the table.

Where the shared lines live: the vLLM container definition (entrypoint, probes, volumes, security context, the capability probe blocks) is written once in `config-llm-template` and copied, not inherited, into the decode, prefill and worker variants. The decode variants add the routing sidecar and the P/D labels; the prefill variant puts the same content under `spec.prefill`; the worker variants add `spec.worker`. So a change to the shared part is a change to every copy.

## Keep the variants in step

- Grep every sibling preset for the line you change. Why: there is no inheritance between presets; a line present in one and missing in another is a per-shape behaviour difference.
- Apply the change with the right spec path: `.Spec.Prefill.Template` for prefill, `.Spec.Worker` for workers, both leader and worker for multi-node.
- Use the same image tag in every variant that ships the container. Why: a leader and a worker sidecar on different versions give version-dependent routing inside one service.
- Prefer an injection in `config_merge.go` over a forked template for a runtime-specific addition. Copy `stripModalityRouteRules`.
- Pin the invariant across variants in `config_presets_test.go` (`TestPresetRenderingInvariants`, `TestVLLMPresetsDeclareModelArgsSlot` are the shape).
- Decide for every new `spec.annotations` key whether it is workload config or routing config. Register a routing-only key in the router's exclusion list. Why: otherwise it reaches the pod template.

## Flags and probes

- Put a new vLLM flag inside a capability probe block of `config-llm-template.yaml`: `ACCESS_LOG_ARGS`, `SHUTDOWN_TIMEOUT_ARGS`, `KV_TRANSFER_ARGS`. Name the vLLM PR that introduced the flag in the comment. Why: the image a user pins may predate the flag.
- Add a sidecar or EPP flag only when the pinned image ships it. Check with `podman run --rm --network=none <image> --help`. The `Image CLI flag drift` workflow replays the flags the presets, samples and fixtures pass against a bumped image; a flag it rejects is one tenants' templates carry too, so the controller migrates it (see `GATES.md`, operand flags), the preset does not just drop it.
- Fail fast when the user asked for a feature the image cannot provide. Do not skip the flag in silence.
- Give scheduler and tokenizer templates the same TLS flags and cert mounts as `config-llm-template.yaml`.

## Security context

- Start a new CPU-only container from the `config-llm-tokenizer.yaml` security context: `readOnlyRootFilesystem: true`, `runAsNonRoot: true`, an emptyDir for `/tmp`.
- Set `runAsNonRoot: true`. Never set `runAsUser`. Why: OpenShift's restricted SCC assigns the UID from the namespace range and rejects a fixed one.
- Add a YAML comment with the reason for any weaker security setting.
- Never relax a value a review thread settled on.

## Images and versions

Operand images are pinned by tag in the presets, and a flag a preset passes must exist in the image the user is pinned to. A tag that does not exist, or a flag the image predates, fails at pod start, after the preset shipped.

- Run `podman manifest inspect` for every tag you change. Why: llm-d router images run one version ahead of the llm-d release, and `v0.9.0` and `0.9.0` are different tags.
- Use the same tag in every preset that ships the container.
- Key a migration on a per-container version key. Why: `app.kubernetes.io/version` on a multi-container template is ambiguous.
- Change a version in `go.mod`, then run `hack/setup/scripts/generate-versions-from-gomod.py` to regenerate `kserve-deps.env`.
- Bump the versions an operand release was built against together with it: Gateway API, GIE and the llm-d CRD bundle in `kserve-deps.env`, from the operand's release notes or `go.mod`. Why: llm-d-router 1.0.0 moved Gateway API to a 1.6 release, and the pins are consumed together at install time. Take the exact version from the operand's `go.mod` at its tag, not from a review comment.
- Do not commit a `TODO: bump` or `main` placeholder. Open an issue instead.
- Check the preset's flags against the new version when you bump an image a preset also pins.
- Put a one-line comment with the source next to every `resources:` or probe block you add or change. The source is the llm-d guide at a pinned version, or a measurement.

## Routes

The route template renders once per model and again per LoRA adapter, against Gateway API limits, and on OpenShift every path is authenticated by a policy that lists paths. A rule added here can exhaust the budget or go out unauthenticated.

- Count the rules and matches in `config-llm-router-route.yaml` after LoRA expansion before you add a rule. The budget is 16 rules, 64 matches per rule, 128 matches per route, and one `PathPrefix` per rule with `replacePrefixMatch`.
- Send catch-all rules to the Service. Never let an InferencePool-bound path fall into the catch-all.
- Mirror every new path in the ODH AuthPolicy template. Why: otherwise it is unauthenticated on OpenShift. Say so in the PR body.

## Naming

Preset names are what services pin, what samples resolve to and what downstream presets build on. A name taken from another component's taxonomy does not transfer.

- Name a preset `kserve-config-llm-<component>-<role>`. The role is what it means for kserve: `default`, `default-pd`, `modality`. Do not use the upstream guide's label.
- Name the primary container in every preset `main`.
- Keep samples under `docs/samples/llmisvc/` resolving to one of these presets, with only the delta. See `docs/samples/llmisvc/AGENTS.md`.

## Before you open the PR

Have a fresh pass, another agent if you can, try to refute each statement against the diff:

- The default render is byte-identical when a new variable is unset. Any restart of existing pods is deliberate and in the release note.
- The change is in every sibling preset with the right spec path and the same image tag, or injected once in `config_merge.go`.
- New vLLM flags sit inside the capability probe blocks. Sidecar and EPP flags exist in the pinned image.
- Every image tag exists. `kserve-deps.env` was regenerated, not hand-edited.
- For an image bump: `make check-image-flag-drift` passes, the previous version's tests stay beside the new ones, and the dashboards under `docs/samples/llmisvc/e2e-gpt-oss/grafana/` carry any metric rename.
- Route rules fit the 16/64/128 budget after LoRA expansion. Every new path is mirrored in the ODH AuthPolicy.
