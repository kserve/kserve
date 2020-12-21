## Metrics and Monitoring

> Getting started with Prometheus based monitoring of model versions defined in InferenceService resource objects.

The initial focus of this documentation is monitoring and querying system metrics such as mean/tail latency and error rates. Documentation for ML/business metrics will be added in the future.

### Table of Contents
1. [Prometheus installation](#prometheus-installation)
2. [Exposing the Prom UI](#exposing-the-prom-ui)
3. [Example 1: Prom queries with InferenceService v1beta1 API](#example-1-prom-queries-with-inferenceservice-v1beta1-api)
4. [Example 2: Prom queries with InferenceService v1alpha2 API](#example-2-prom-queries-with-inferenceservice-v1alpha2-api)
5. [Metrics and AI-driven live experiments, progressive delivery, and automated rollouts](#metrics-and-ai-driven-live-experiments-progressive-delivery-and-automated-rollouts)

### Prometheus installation

**Prerequisites:** [Kustomize v3](https://kubectl.docs.kubernetes.io/installation/kustomize/).

Install Prometheus operator.

```shell
cd kfserving
kustomize build docs/samples/metrics-and-monitoring/prometheus-operator | kubectl apply -f -
```

### Exposing the Prom UI

### Example 1: Prom queries with InferenceService v1beta1 API

### Example 2: Prom queries with InferenceService v1alpha2 API

### Metrics and AI-driven live experiments, progressive delivery, and automated rollouts
See [iter8-kfserving project](https://github.com/iter8-tools/iter8-kfserving).