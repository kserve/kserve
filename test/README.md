# Testing
Please refer to [unit and e2e tests guide](https://github.com/kserve/website/blob/main/docs/developer-guide/index.md#running-unitintegration-tests)

## E2E worker namespaces

Tests using `test_namespace` reuse one namespace per pytest worker. Each session
gets a fresh namespace name, so consecutive pytest runs cannot collide with a
previous session's namespace while Kubernetes is still deleting it. Setup copies
storage secrets and namespaced `ServingRuntime` definitions from
`KSERVE_SEED_NAMESPACE` (defaults to `KSERVE_TEST_NAMESPACE`, then
`kserve-ci-e2e-test`). The default test installation uses `ClusterServingRuntime`
resources, which are available across namespaces and are not copied or modified
by this fixture. If a test environment instead uses namespaced `ServingRuntime`
resources, install them in the seed namespace before running the suite; each
worker receives a copy once per session.

Namespace isolation separates test resources and their cleanup. It does not
install or update cluster-scoped CRDs; the test environment must install the
CRDs and controller for the revision under test before running the suite.

Worker namespaces inherit the seed's Istio injection and pod-security labels.
Platform-specific mesh membership, network policies, and additional runtime
dependencies (such as ConfigMaps, custom service accounts, or image pull secrets)
must also be provisioned for worker namespaces if required by the environment.
Per-test cleanup removes InferenceServices and TrainedModels; the worker namespace
and its runtimes are deleted at session teardown, including after setup failure.
`SKIP_RESOURCE_DELETION=true` preserves these resources for debugging.

`SKIP_DELETION_ON_FAILURE`, which `run-e2e-tests.sh` sets, keeps the namespace
of a failed test only where tests get their own namespace (`llmisvc`). A worker
namespace is reused by the next test, so failures there are captured instead.

## E2E failure diagnostics

When a test using `test_namespace` fails in setup or call, its namespace state
is captured before per-test cleanup deletes it. Collection runs only when
`E2E_DIAGNOSTICS_DIR` is set; `run-e2e-tests.sh` and `run-qpext-test.sh` set it
to `${ARTIFACT_DIR:-/tmp}/e2e-diagnostics-<marker>`, and the
`upload-test-results` action uploads it with the JUnit and JSON reports.

Each failure gets `<worker>/<test node ID>-<start time>/` with:

- `meta.json`: test node ID, worker, namespace, run ID and attempt, and the
  tested commit with its parents (CI merges the base branch after checkout, so
  this is not the PR head)
- YAML of InferenceServices, TrainedModels, Knative Services, Configurations,
  Revisions and PodAutoscalers, Deployments, ReplicaSets and Pods
- namespace events, `kubectl describe pods`, current logs of every container
  (init containers included) and previous logs of restarted ones
- `kubectl describe nodes`, the node image inventory, and KServe and Knative
  controller logs since the test started
- `errors.txt` for commands that failed or timed out

Every command has a timeout, logs are byte-capped, and one failure has a 180 s
budget. Collection errors never change the test result. A runner that is killed
before the fixture teardown runs (a hard cancel or step timeout) leaves no
diagnostics.

Run the e2e harness unit checks without a cluster (`make e2e-collect` runs
them too):

```sh
python -m pytest test/test_e2e_namespace.py test/test_e2e_diagnostics.py
```
