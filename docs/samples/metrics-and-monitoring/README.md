# Metrics and Monitoring

> Getting started with Prometheus based monitoring of model versions defined in InferenceService resource objects.

The initial focus of this documentation is on monitoring and querying system metrics such as mean/tail latency and error rates. Documentation for ML/business metrics will be added in the future.

# Table of Contents
1. [Installing Prometheus](#installing-prometheus)
2. [Accessing Prom UI](#accessing-prom-ui)
3. [Querying Prometheus](#querying-prometheus)
4. [Metrics-driven live experiments and progressive delivery](#metrics-driven-live-experiments-and-progressive-delivery)
5. [Removal](#removal)

## Installing Prometheus

**Prerequisites:** Kubernetes cluster and [Kustomize v3](https://kubectl.docs.kubernetes.io/installation/kustomize/).

Install Prometheus using Prometheus Operator.

```shell
cd kfserving
kustomize build docs/samples/metrics-and-monitoring/prometheus-operator | kubectl apply -f -
kubectl wait --for condition=established --timeout=120s crd/prometheuses.monitoring.coreos.com
kubectl wait --for condition=established --timeout=120s crd/servicemonitors.monitoring.coreos.com
kustomize build docs/samples/metrics-and-monitoring/prometheus | kubectl apply -f -
```

## Accessing Prom UI

```shell
kubectl port-forward service/prometheus-operated -n kfserving-monitoring 9090:9090
```

You should now be able to access Prometheus UI in your browser at http://localhost:30900.

## Querying Prometheus

## Metrics-driven live experiments and progressive delivery
See [iter8-kfserving](https://github.com/iter8-tools/iter8-kfserving).

## Removal
Remove Prometheus and Prometheus Operator as follows.
```shell
cd kfserving
kustomize build docs/samples/metrics-and-monitoring/prometheus | kubectl delete -f -
kustomize build docs/samples/metrics-and-monitoring/prometheus-operator | kubectl delete -f -
```

