# Pull request description examples

Right-sized descriptions for `.github/PULL_REQUEST_TEMPLATE.md`. Each stays within the template's word budget (150 for a bug fix, 300 for a feature), and a reviewer gets the point in under a minute. The checklist is left out to keep them short.

## Bug fix

Adapted from #6327. The symptom users saw, the root cause in two sentences, the fix, and a test that fails on master.

~~~markdown
fix(python): send a single Content-Type for binary v2 REST requests

**What this PR does / why we need it**:

When a transformer forwards a v2 inference request carrying binary data to a REST v2 predictor, the predictor rejects it with `422 Unprocessable Entity` / `JSON decode error: Extra data`.

The model always passes a JSON `Content-Type`, and for binary requests the REST client added `application/octet-stream` under a differently cased key. Both went out, so the server parsed the binary tensor data as JSON. Binary requests now send exactly one `Content-Type` and leave the caller's headers untouched.

**Feature/Issue validation/testing**:

A new test sends a binary request with a transformer's headers to the in-process v2 server. It fails on master with the 422 and passes here.

**Release note**:

```release-note
Fixed transformers failing with 422 when forwarding binary-encoded v2 REST inference requests to the predictor.
```
~~~

## Feature

Rewritten from #5916. What users can do now and why they need it, a short example, then the decisions a reviewer should weigh in on and whether an upgrade changes anything.

~~~markdown
feat(llmisvc): add rolloutStrategy for controlling rolling updates

**What this PR does / why we need it**:

A single-node LLMInferenceService rolls with the Deployment defaults, which start extra pods before old ones go away. On a cluster without spare GPUs those pods wait for hardware that isn't there, and the rollout can stall.

`rolloutStrategy` sets `maxSurge` and `maxUnavailable` for the main workload and, separately, for prefill. With no spare hardware, an old pod frees its GPU before its replacement is scheduled:

```yaml
spec:
  rolloutStrategy:
    maxSurge: 0
    maxUnavailable: 1
```

With spare capacity, a larger `maxSurge` finishes the rollout in fewer rounds. Both Deployments and LeaderWorkerSets honour it.

**Which issue(s) this PR fixes**:

Fixes #5452

**Feature/Issue validation/testing**:

Unit tests cover validation, v1alpha1 conversion, and how the strategy lands on Deployments and LeaderWorkerSets. On a GPU cluster with no spare capacity, a rollout with `maxSurge: 0` and `maxUnavailable: 1` completed, and patching the field on a live service updated its Deployment.

**Special notes for your reviewer**:

- When the field is omitted, the Kubernetes and LeaderWorkerSet defaults apply rather than a KServe default. Existing services see no change, but single-node and multi-node services keep rolling differently (25%/25% vs 1/0).
- The webhook rejects `maxSurge` and `maxUnavailable` both set to zero. Kubernetes would refuse that Deployment anyway, but only when the controller writes it, so the error would show up at reconcile time instead of on apply.

The field is new and optional. Services that don't set it are unchanged, and nothing rolls on upgrade.

**Release note**:

```release-note
LLMInferenceService accepts `spec.rolloutStrategy` and `spec.prefill.rolloutStrategy` with `maxSurge` and `maxUnavailable`, so rolling updates can run without spare GPUs or finish faster with surge capacity.
```
~~~

## Refactor

From #6311, unchanged apart from the checklist. Why the refactor is worth it, one number, and a plain statement that upgrades roll nothing. The only user-visible change gets one sentence.

~~~markdown
refactor(llmisvc): split pod template rendering from workload builders

**What this PR does / why we need it**:

Splits each workload builder into a pod template renderer and a thin Deployment/LeaderWorkerSet wrapper. The DisaggregatedSet backend (#6330) now uses these shared renderers instead of its own hand-synced copies. All backends render pods from the same code, which removes about 370 lines of duplication.

No change in the objects the controller creates: pod templates, revisions, rollouts, restart policies and label/annotation propagation are identical to master, so upgrades trigger no rollouts. The only change users can see is clearer wording when attaching model artifacts fails. The error now names the template (for example `main pod template`) instead of a "deployment" that doesn't exist on the DisaggregatedSet path.

**Special notes for your reviewer**:

The DisaggregatedSet role functions are kept as thin adapters, so their callers are unchanged.

**Release note**:

```release-note
NONE
```
~~~
