# Common issues and solutions

This document compiles common issues that users may encounter and provides solutions for troubleshooting.

## Issue list
  - [CRD Annotations Too Long](#issue-crd-annotations-too-longissue-crd-annotations-too-long)
  - [Failed ci - Verify Generated Code / verify-codegen](#issue-failed-ci---verify-generated-code--verify-codegen)
## Issue detail

**New issues should be documented above existing issues.**
<!-- 
Issue Template
### Title
- Summary: A brief description of the issue (1-2 sentences).
- Symptoms: Key symptoms or error messages.
- Cause: Short description of the root cause.
- Resolution: Concise steps to resolve the issue.
- Author: Name/Email of the person who reported the issue.
- Date: Date when the issue was documented.
- Related Issues/PRs: Links or references to any related GitHub issues or PRs.
-->
---
### CRD Annotations Too Long
- Summary: Error when applying CRDs due to excessive annotation size.
- Symptoms: 
  - error msg:
    ~~~
    "metadata.annotations: Too long: must have at most 262144 bytes."
    ~~~
- Cause: CRD annotations exceed the Kubernetes size limit.
- Resolution: 
  - Reduce annotation size or use server-side apply.
  - ex) `kubectl apply --server-side=true -k ./config/default`
- Author: Jooho Lee/jlee@redhat.com
- Date: 2024-08-20
- Related Issues/PRs: [#3487](https://github.com/kserve/kserve/issues/3487), [#3144](https://github.com/kserve/kserve/pull/3144), [#3877](https://github.com/kserve/kserve/pull/3877)

---
### Failed ci - Verify Generated Code / verify-codegen
- Summary: when PR has a new api added for CRD, CI(Verify Generated Code / verify-codegen) might fail
- Symptoms: 
  - error msg in ci:
    ~~~
    "kserve/kserve is out of date. Please run make generate"
    ~~~
- Cause: 
  - There are some diffs in openapi generated file: pkg/apis/serving/v1beta1/openapi_generated.go. (inferenceservice crd case)
- Resolution: 
  - run these make targets in order
    ~~~
    make generate
    make manifests
    ~~~
- Author: Jooho Lee/jlee@redhat.com
- Date: 2024-08-19
- Related Issues/PRs: X
