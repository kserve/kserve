# End-to-end tests

Read the root `AGENTS.md` first. The suites are pytest. The workflows under `.github/workflows/` run them on minikube or kind through `test/scripts/gh-actions/`. `llmisvc/README.md` documents the config composition pattern and the markers.

## Use the harness

The llmisvc suite composes each case from config fragments; the harness retries only assertion errors and collects diagnostics on failure. A test written outside it hides a terminal state behind a timeout, loses the logs a maintainer needs, and cannot say which route rule it covers.

- Add new LLMInferenceService coverage as a `TestCase` row in `llmisvc/test_llm_inference_service.py`.
- Use `wait_for()`, `maybe_delete_llmisvc()` and `collect_diagnostics()` from `llmisvc/diagnostic.py`.
- Compose a row's spec from `base_refs` keys in `llmisvc/fixtures.py:LLMINFERENCESERVICE_CONFIGS`. Examples: `workload-single-cpu`, `model-fb-opt-125m`, `router-managed`. Add a fragment there instead of inline YAML. Why: the service name and the model name derive from the refs.
- Give a reason the table cannot express before you add a new test file, a marker, or a polling loop.
- Raise a non-assertion exception for a terminal state, or check it before the loop. Why: `wait_for()` retries only `AssertionError`. Terminal states: a `Failed` phase, a 404 on a resource you already confirmed, a 409 on create.
- Never turn a missed assertion into `logger.warning`.
- Never write `except Exception: pass` in teardown. Log, then re-raise anything that is not 404.
- Treat a 4xx response as a failure.
- Build fixture objects with factories that take `namespace` and `name`. Do not run a recursive string replace over the config tree.
- Do not inject a header into every request. Put headers, URLs and model names in the `TestCase` row. Why: each test then states which rule of `config/llmisvcconfig/config-llm-router-route.yaml` it covers.
- Put the last observed state in the message of every poller that raises.
- Dump what a maintainer needs on the failure path: pod logs, trace IDs, the download Job's logs.
- Scope Jaeger and metric queries to the service under test.

## Isolation

The suites run in parallel under xdist against one cluster with one `inferenceservice-config`, which the controller re-reads on every reconcile. Shared names and leaked state turn one test's failure into another's.

- Give each test its own labelled namespace through the `test_case` and `test_namespace` fixtures in `llmisvc/namespace.py`.
- Include every parametrize dimension in a generated name. Why: two cases that share a name 409 under xdist.
- Restore `inferenceservice-config`, a preset in the shared namespace, or gateway state in teardown when a test mutates it. Mark such a test `llmisvc_serial`. Why: the controller re-reads the ConfigMap on every reconcile, so a leak affects every later test.
- Wait for an async deletion before you recreate the object.
- Read cluster coordinates from the env variables the `hack/setup` scripts export: namespaces, `GATEWAYCLASS_NAME`, the registry URL. Grep `hack/` before you add a variable name.
- Never hardcode `kserve`, `monitoring` or `keda` as a namespace.

## What a test must prove

A test that asserts the shape of a Deployment passes when the model never serves, a fake that accepts any argument passes on a typo, and a timeout widened without a measured cause hides the race it was meant to catch.

- Assert the behaviour through the gateway or the model: a real response, pods that scale, an adapter that is served. Do not assert the shape of a Deployment or a ScaledObject.
- Make a fake assert its lookup arguments. Why: otherwise a typo in a route or rule name passes.
- Make the fixture take the exact path the change is about: a `pvc://` adapter for a PVC fix, Istio for regex precedence, both CI gateway jobs for rule ordering or EPP attachment.
- Use the smallest model. Normally this is the `facebook/opt-125m` copy that `test/scripts/gh-actions/setup-kserve.sh` seeds into SeaweedFS.
- Mark CPU tests `cluster_cpu`. No GPU job exists upstream.
- Name the failing run and the race in a flakiness fix. Show that the new wait observes the right object.
- Move a timeout only with a measurement. A blanket `|| true`, a sleep, or a wider timeout without a linked failure is not a fix.
- Call `test/scripts/gh-actions/run-e2e-tests.sh "<marker expr>" <parallelism> <istio|envoy-gateway>` with all three arguments. Why: the third argument defaults to `istio`, so an envoy-gateway run without it tests the wrong layer.
- Set `SKIP_DELETION_ON_FAILURE=true` to keep the namespaces of failed tests, or `SKIP_RESOURCE_DELETION=true` to keep everything. Make every new teardown honour both.
- Run the suite locally the way the workflow does before you push: `/kserve-llmisvc-e2e`, or minikube at CI's Kubernetes version for anything that touches volumes. Why: kind hides kubelet bugs that CI sees.
- Check that the junit artifact shows the test ran. Why: CI renames presets with a prefix, so an exact-name lookup skips in silence.

## Markers and jobs

CI selects tests by marker and runs `pull_request_target` workflows with a write token on the base ref. A marker registered twice or nowhere fails the run under `--strict-markers`; a workflow that executes PR-head code with that token leaks the credential.

- Register static markers in `pytest.ini`. `autoscaling_<variant>` markers derive from `test_llm_autoscaling_<variant>.py` filenames in `llmisvc/conftest.py:pytest_configure`.
- Register a marker in exactly one place. Why: `--strict-markers` fails on a marker applied by hand and registered nowhere.
- Select tests in jobs with positive marker expressions such as `llmisvc_core`. Never with `and not x and not y`.
- Keep a pure-function test from inheriting `llmisvc_serial` or a cluster marker from its class.
- In a `pull_request_target` workflow, never check out or execute PR-head code. Scope `permissions` to the step. Add a comment that says why the step needs the token and why that is safe. These workflows run with a write token on the base ref: `go.yml` coverage comment, `rerun.yml`, `pr-style-check.yml`, reviewer assignment, lgtm removal.
- Compose a new workflow job from `.github/actions/{minikube-setup,kserve-dep-setup,load-docker-images,upload-test-results}`. Add an input to the action instead of pasting steps or misusing an existing input.
- Pin every `uses:` to a SHA. Keep the `pull_request` trigger and the path filters. Register a new image in `kserve-images.env`.
- Verify post-conditions in scripts under `hack/` and `test/scripts/`: grep the patched output, check the image exists. Exit 1 on a miss. Why: a silent no-op surfaces later as an unrelated e2e failure.

## Before you open the PR

Have a fresh pass, another agent if you can, try to refute each statement against the diff:

- New llmisvc coverage is a `TestCase` row that uses `wait_for()` and `collect_diagnostics()`. Pollers fail fast on terminal states and never swallow errors.
- Each test has its own namespace. Names include every parametrize dimension. Shared state is restored in teardown, with `llmisvc_serial` when it cannot be.
- The test asserts behaviour through the gateway or the model, on the smallest model, marked `cluster_cpu`.
- Markers are registered in exactly one place. Jobs select with positive expressions.
- You ran it locally the way the workflow does, and the junit artifact shows it ran. A flakiness fix names the failing run and the race.
