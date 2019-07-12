# Knative Serving API spec

This directory contains the specification of the Knative Serving API, which is
implemented in [`pkg/apis/serving/v1alpha1`](/pkg/apis/serving/v1alpha1) and
verified via [the conformance tests](/test/conformance).

**Updates to this spec should include a corresponding change to
[the API implementation](/pkg/apis/serving/v1alpha1) and
[the conformance tests](/test/conformance).**
([#780](https://github.com/knative/serving/issues/780))

Docs in this directory:

- [Motivation and goals](motivation.md)
- [Resource type overview](overview.md)
- [Knative Serving API spec](spec.md)
- [Error conditions and reporting](errors.md)
- [Sample API usage](normative_examples.md)
