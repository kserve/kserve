# Conformance tests

- [Running conformance tests](../README.md#running-conformance-tests)

## Adding conformance tests

Knative Serving conformance tests
[can be run against any implementation of the Knative Serving API](#requirements)
to ensure the API has been implemented consistently. Passing these tests
indicates that apps and functions deployed to this implementation could be
ported to other implementations as well.

_The precedent for these tests is
[the k8s conformance tests](https://github.com/cncf/k8s-conformance)._

These tests use [the test library](../adding_tests.md#test-library).

### Requirements

The conformance tests should **ONLY** cover functionality that applies to any
implementation of the API.

The conformance tests **MUST**:

1. Provide frequent output describing what actions they are undertaking,
   especially before performing long running operations. Please see the
   [Log section](../adding_tests.md#output-log) for detailed instructions.
2. Follow Golang best practices.
3. Not require any specific file system permissions to run or require any
   additional binaries to be installed in the target environment before the
   tests run.
4. Not depend on any k8s resources outside of those added by Knative Serving OR
   they should provide flags that allow the test to run without access to those
   resources.
