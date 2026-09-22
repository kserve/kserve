# Testing
Please refer to [unit and e2e tests guide](https://github.com/kserve/website/blob/main/docs/developer-guide/index.md#running-unitintegration-tests)

## E2E worker namespaces

Tests using `test_namespace` reuse one namespace per pytest worker. Each session
gets a fresh namespace name, so consecutive pytest runs cannot collide with a
previous session's namespace while Kubernetes is still deleting it. Setup copies
storage secrets and namespaced `ServingRuntime` definitions from
`KSERVE_SEED_NAMESPACE` (defaults to `KSERVE_TEST_NAMESPACE`, then
`kserve-ci-e2e-test`). Install the test runtimes in that seed namespace before
running the suite. Each worker gets its own copy once per session, so ODH tests
can use namespaced runtimes without installing `ClusterServingRuntime` resources.
Upstream clusters with only cluster-scoped runtimes need no runtime copies.

Worker namespaces inherit the seed's Istio injection and pod-security labels.
Platform-specific mesh membership, network policies, and additional runtime
dependencies (such as ConfigMaps, custom service accounts, or image pull secrets)
must also be provisioned for worker namespaces if required by the environment.
Per-test cleanup removes InferenceServices and TrainedModels; the worker namespace
and its runtimes are deleted at session teardown, including after setup failure.
`SKIP_RESOURCE_DELETION=true` preserves these resources for debugging.

Run the namespace provisioning unit checks without a cluster:

```sh
python -m unittest test.test_e2e_namespace
```
