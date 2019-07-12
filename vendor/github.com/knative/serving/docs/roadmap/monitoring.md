# 2018 Roadmap for Monitoring and Logging

This document captures what we hope to accomplish in 2018 in Monitoring and
Logging areas for Knative Serving.

## Overview

We will provide distinct experiences for
[operator personas](../product/personas.md#operator-personas),
[developer personas](../product/personas.md#developer-personas) and
[contributors](../product/personas.md#contributors).

### Operator Capabilities

- Provide default collection of cluster logs and metrics from infrastructure
  components such as Kubernetes.
- Provide default dashboards and interfaces for viewing cluster logs and
  metrics.
- Auto-scale, upgrade and maintain the default logging, metrics, alerting and
  tracing backends.
- Operators can set custom alerts on cluster events.
- Operators can fine tune of scale, performance and features of the default
  logging, metrics, alerting and tracing backends.
- Operators can retrieve a list of all components emitting logs or metrics using
  a CLI.
- Operators can "tail" logs and metrics using a CLI for a specific component.
- Operators can install extensions that forward logs and metrics to different
  backends (e.g. Stack Driver).

### Developer Capabilities

- Provide default collection of logs, metrics, and request traces.
- Provide default dashboards and interfaces for viewing logs, metrics and
  traces, and for setting alerts on the same.
- Developers can set custom application and function alerts.
- Developers can create shared dashboards for logs and metrics for applications
  and functions.
- Developers can retrieve a list of all components they have access to that are
  emitting logs and/or metrics using a CLI.
- Developers can "tail" logs and metrics using a CLI for any component they have
  access to.

### Contributor Capabilities

- Contributors can write extensions and translate logs and metrics into the
  format for different loggings and metrics stores (e.g. StackDriver).

## Basics

### Milestones: M3 and M4

In this phase, we will enable a shared infrastructure where everyone has access
to all data. No personas specific experience or access will be provided.

The following items will be installed and secured in a cluster by default, but
we will provide the ability to replace or remove these in a later milestone.

- Prometheus
- Alert Manager
- Prometheus Operator
- Grafana
- ElasticSearch
- Kibana
- Zipkin
- Fluentd

Logs from the following locations will be collected:

- stderr & stdout for all application and function containers
- Build logs

Following metrics will be collected:

- Envoy, Istio Mixer (per request metrics), Istio Pilot
- Node and pod level metrics (CPU, memory, disk and network)
- Knative Serving controller metrics

Request logs from Istio proxy, user applications and user functions will be
collected by Zipkin.

## Developer Contracts

### Milestones: M4 and M5

In this phase, we will define and implement features for the developer persona.

- [M4 & M5] Define and implement developer contracts for logging, metrics,
  alerting and tracing.
- [M4] Write step-by-step guidelines for developers to debug issues throughout
  the lifecycle of their applications and functions.
- [M4] Provide developer samples written in Golang. Support for other languages
  will come in a later phase.
- [M5] Implement the developer CLI to list components and tail logs, metrics and
  traces.

## Operator Contracts

### Milestones: M6 and M7

In this phase, we will define and implement features for the operator persona.

- [M6 & M7] Define and implement operator contracts.
- [M6] Write step-by-step guidelines for operators to debug issues in the
  cluster.
- [M7] Deploy operator specific instances of the default backends to separate
  access of operators vs developers.
- [M7] Implement the operator CLI to list components and tail logs and metrics.

## Contributor Contracts

### Milestones: M8

In this phase, we will define and implement the features for the contributor
persona.

- [M8] Define and implement contracts for plugging in custom logging, metrics,
  alerting and tracing backends. We will not provide maintenance, rollout
  processes, etc for third-party monitoring, logging, or tracing extensions,
  though we may maintain a "contrib" directory for such contributions.
- [M8] Add an extension for one managed solution (e.g. Stack Driver).

## M9 and Onwards

- Allow namespace specific instances of default backends for namespace level
  access control.
- Implement auto-scaling of the default backends.
- Implement upgrading of the default backends.
- Implement maintenance of the default backends (data retention, daily index
  creations, etc).
- Provide developer samples written in Node.js, Java, Python, PHP, .Net and
  Ruby.

## Out of Scope for 2018

- Improving the underlying logging, monitoring, and tracing systems to support
  multi-tenancy.
