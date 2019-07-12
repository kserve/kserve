# End to end tests

- [Running e2e tests](../README.md#running-e2e-tests)

## Adding end to end tests

Knative Build e2e tests
[test the end to end functionality of the Knative Build API](#requirements) to
verify the behavior of this specific implementation.

### Requirements

The e2e tests are used to test whether the flow of Knative Build is performing
as designed from start to finish.

The e2e tests **MUST**:

1. Provide frequent output describing what actions they are undertaking,
   especially before performing long running operations.
2. Follow Golang best practices.
