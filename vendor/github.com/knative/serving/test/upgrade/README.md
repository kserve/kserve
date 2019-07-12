# Upgrade Tests

In order to get coverage for the upgrade process from an operator’s perspective,
we need an additional suite of tests that perform a complete knative upgrade.
Running these tests on every commit will ensure that we don’t introduce any
non-upgradeable changes, so every commit should be releasable.

This is inspired by kubernetes
[upgrade testing](https://github.com/kubernetes/community/blob/master/contributors/devel/e2e-tests.md#version-skewed-and-upgrade-testing).

These tests are a pretty big hammer in that they cover more than just version
changes, but it’s one of the only ways to make sure we don’t accidentally make
breaking changes for now.

## Flow

We’d like to validate that the upgrade doesn’t break any resources (they still
respond to requests) and doesn't break our installation (we can still update
resources).

At a high level, we want to do this:

1. Install the latest knative release.
1. Create some resources.
1. Install knative at HEAD.
1. Test those resources, verify that we didn’t break anything.
1. Install the previous release (downgrade).
1. Test those resources (again), verify that we didn’t break anything.

To achieve that, we just have three separate build tags:

1. Install the latest release from GitHub.
1. Run the `preupgrade` tests in this directory.
1. Install at HEAD (`ko apply -f config/`).
1. Run the `postupgrade` tests in this directory.

## Tests

### Service test

This was stolen from the conformance tests but stripped down to check fewer
things.

#### preupgrade

Create a RunLatest Service pointing to `image1`, ensure it responds correctly.

#### postupgrade

Ensure the Service still responds correctly after upgrading. Update it to point
to `image2`, ensure it responds correctly.
