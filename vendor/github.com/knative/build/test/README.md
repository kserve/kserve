# Test

This directory contains tests and testing docs for `Knative Build`:

- [Unit tests](#running-unit-tests) currently reside in the codebase alongside
  the code they test
- [End-to-end tests](#running-end-to-end-tests), of which there are two types:

## Running unit tests

To run all unit tests:

```bash
go test ./...
```

_By default `go test` will not run [the e2e tests](#running-end-to-end-tests),
which need [`-tags=e2e`](#running-end-to-end-tests) to be enabled._

## Running end to end tests

### Environment requirements

Setting up a running `Knative Build` cluster.

1. A Kubernetes cluster v1.10 or newer with the `MutatingAdmissionWebhook`
   admission controller enabled. `kubectl` v1.10 is also required.
   [see here](https://www.knative.dev/docs/install/knative-with-any-k8s/)

1. Configure `ko` to point to your registry.
   [see here](https://github.com/knative/build/blob/master/DEVELOPMENT.md#one-time-setup)

### Go e2e tests

To run [the Go e2e tests](./e2e), you need to have a running environment that
meets [the e2e test environment requirements](#environment-requirements) and
[stand up a version of this controller on-cluster](https://github.com/knative/build/blob/master/DEVELOPMENT.md#standing-it-up).

Finally run the Go e2e tests with the build tag `e2e`.

```bash
go test -v -tags=e2e -count=1 ./test/e2e/...
```

`-count=1` is the idiomatic way to bypass test caching, so that tests will
always run.

### YAML e2e tests

To run the YAML e2e tests, you need to have a running environment that meets
[the e2e test environment requirements](#environment-requirements).

```bash
./test/e2e-tests-yaml.sh
```

### One test case

To run one e2e test case, e.g. TestSimpleBuild, use
[the `-run` flag with `go test`](https://golang.org/cmd/go/#hdr-Testing_flags):

```bash
go test -v -tags=e2e -count=1 ./test/e2e/... -run=<regex>
```
