# End to end tests

- [Running e2e tests](../README.md#running-e2e-tests)

## Adding end to end tests

Knative Serving e2e tests
[test the end to end functionality of the Knative Serving API](#requirements) to
verify the behavior of this specific implementation.

These tests use [the test library](../adding_tests.md#test-library).

### Requirements

The e2e tests are used to test whether the flow of Knative Serving is performing
as designed from start to finish.

The e2e tests **MUST**:

1. Provide frequent output describing what actions they are undertaking,
   especially before performing long running operations. Please see the
   [Log section](../adding_tests.md#output-log) for detailed instructions.
2. Follow Golang best practices.
