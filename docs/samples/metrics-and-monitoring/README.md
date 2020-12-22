## Metrics and Monitoring

> Getting started with Prometheus based monitoring of model versions defined in InferenceService resource objects.

The initial focus of this documentation is monitoring and querying system metrics such as mean/tail latency and error rates. Documentation for ML/business metrics will be added in the future.

### Table of Contents
1. [Installing Prometheus](#installing-prometheus)
2. [Accessing Prom UI](#accessing-prom-ui)
3. [Querying Prometheus (InferenceService v1beta1)](#querying-prometheus-inferenceservice-v1beta1)
4. [Querying Prometheus (InferenceService v1alpha2)](#querying-prometheus-inferenceservice-v1alpha2)
5. [Metrics and AI-driven live experiments and progressive delivery](#metrics-and-ai-driven-live-experiments-progressive-delivery-and-automated-rollouts)

### Installing Prometheus

**Prerequisites:** Kubernetes cluster and [Kustomize v3](https://kubectl.docs.kubernetes.io/installation/kustomize/).

Install Prometheus operator.

```shell
cd kfserving
kustomize build docs/samples/metrics-and-monitoring/prometheus-operator | kubectl apply -f -
kubectl wait --for condition=established --timeout=120s crd/prometheuses.monitoring.coreos.com
kubectl wait --for condition=established --timeout=120s crd/servicemonitors.monitoring.coreos.com
kustomize build docs/samples/metrics-and-monitoring/prometheus | kubectl apply -f -
```

### Accessing Prom UI

```shell
kubectl port-forward service/kfserving-prometheus -n kfserving-monitoring 30900:9090
```

You should now be able to access Prometheus UI in your browser at http://localhost:30900.

### Querying Prometheus (InferenceService v1beta1)

### Querying Prometheus (InferenceService v1alpha2)

### Metrics and AI-driven live experiments and progressive delivery
See [iter8-kfserving](https://github.com/iter8-tools/iter8-kfserving).