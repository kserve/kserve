# Knative Serving Changelog

All notable changes to Knative Serving will be documented in this file.

The format is based on [Keep a Changelog](http://keepachangelog.com/en/1.0.0/)
and this project adheres to
[Semantic Versioning](http://semver.org/spec/v2.0.0.html).

When adding a changelog entry, link to the most relevant issue or PR for more
details.

## [Unreleased]

### Added

- Basic API resources Revision, Configuration, and Route.
- Integration with the Build API for on-demand container builds.
  [#36](https://github.com/knative/serving/pull/36)
- Autoscaling of Revision deployments.
  [#229](https://github.com/knative/serving/pull/229)
- Integration with Istio ingress routing and naming.
  [#313](https://github.com/knative/serving/issues/313)
- Integration with Istio proxy sidecar.
  [#112](https://github.com/knative/serving/issues/112)
- Tracing via Zipkin. [#354](https://github.com/knative/serving/pull/354)
- Logging via Fluentd with log indexing via Elasticsearch.
  [#327](https://github.com/knative/serving/pull/327)
- Metrics via Prometheus.[#189](https://github.com/knative/serving/pull/189)

### Changed

### Removed

<!-- To create a new release:

1. Change [Unreleased] to the released version number and add a release date.
   Example: ## [0.1.0] - 2018-04-01

2. Copy the following template above the just released version:

## [Unreleased]
### Added

### Changed

### Removed

-->
