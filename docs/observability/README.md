# KFServing Observability

The [Knative Observability](https://github.com/knative/observability) project is deprecated and no longer ships releases. So for the time being, it is necessary to look deeper for metrics regarding deployed `InferenceServices`.

## Istio / Prometheus

The default Prometheus deployment is configured to collect metrics from all Envoy proxies running in the cluster, augmenting each metric with a set of labels about their origin (instance, pod, and namespace).

### Workload Aggregation

The Istio documentation describes Prometheus rules which will provide aggregated metrics by workload. In their words, "In Kubernetes, a workload typically corresponds to a Kubernetes deployment, while a workload instance corresponds to an individual pod". By collecting and viewing these metrics with Prometheus, one can get a good sense of the state of the services requests and its volume of traffic.

These properties are available in policy and telemetry configuration using the following attributes:

```
source.workload.name, source.workload.namespace, source.workload.uid
destination.workload.name, destination.workload.namespace, destination.workload.uid
```

https://istio.io/latest/docs/ops/best-practices/observability/#workload-level-aggregation-via-recording-rules
