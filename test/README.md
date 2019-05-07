# Test

This directory contains tests and testing docs for `KFServing`:

- [Unit tests](#running-unit-tests) currently reside in the codebase alongside
  the code they test
- [End-to-end tests](#running-end-to-end-tests):
  - They are in [`/test/e2e`](./e2e)

## Prerequisite
`kfserving-controller-manager` has a few integration tests which requires mock apiserver
and etcd, they get installed along with [`kubebuilder`](https://book.kubebuilder.io/getting_started/installation_and_setup.html).

## Running unit/integration tests

To run all unit tests:

```bash
make test
```

## Running end to end tests

To run [the e2e tests](./e2e), you
need to have a running environment that meets the e2e test environment requirements. (@TODO)


