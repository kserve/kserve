# Metrics and Monitoring

> Getting started with Prometheus based monitoring of model versions defined in InferenceService resource objects.

# Table of Contents
1. [Install Prometheus](#install-prometheus)
2. [Access Prometheus Metrics](#access-prometheus-metrics)
3. [Metrics-driven experiments and progressive delivery](#metrics-driven-experiments-and-progressive-delivery)
4. [Removal](#removal)

## Install Prometheus

**Prerequisites:** Kubernetes cluster and [Kustomize v3](https://kubectl.docs.kubernetes.io/installation/kustomize/).

Install Prometheus using Prometheus Operator.

```shell
cd kfserving
kustomize build docs/samples/metrics-and-monitoring/prometheus-operator | kubectl apply -f -
kubectl wait --for condition=established --timeout=120s crd/prometheuses.monitoring.coreos.com
kubectl wait --for condition=established --timeout=120s crd/servicemonitors.monitoring.coreos.com
kustomize build docs/samples/metrics-and-monitoring/prometheus | kubectl apply -f -
```

## Accessing Prometheus Metrics
1. `kubectl create ns kfserving-test`
2. `cd docs/samples/v1beta1/sklearn`
2. `kubectl apply -f sklearn.yaml -n kfserving-test`
3. In a separate terminal, send requests. First, follow these instructions to find your ingress IP, host, and service hostname. Then, 
```
while clear; do \
  curl -v \
  -H "Host: ${SERVICE_HOSTNAME}" \
  -d @./iris-input.json \
  http://${INGRESS_HOST}:${INGRESS_PORT}/v2/models/sklearn-iris/infer
  sleep 0.3
done
```
4. In a separate terminal, run 
```shell
kubectl port-forward service/prometheus-operated -n kfserving-monitoring 9090:9090
```
5. Access Prometheus UI in your browser at http://localhost:9090
6. Access the number of prediction requests to the sklearn model, over the last 60 seconds as follows.
![Request count](requestcount.png)
7. Access the mean latency for serving prediction requests for the same model as above, over the last 60 seconds as follows.
![Request count](requestlatency.png)

## Metrics-driven experiments and progressive delivery
See [iter8-kfserving](https://github.com/iter8-tools/iter8-kfserving).

## Removal
Remove Prometheus and Prometheus Operator as follows.
```shell
cd kfserving
kustomize build docs/samples/metrics-and-monitoring/prometheus | kubectl delete -f -
kustomize build docs/samples/metrics-and-monitoring/prometheus-operator | kubectl delete -f -
```

