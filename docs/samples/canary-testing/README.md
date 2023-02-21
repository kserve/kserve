# Canary testing

This tutorial demonstrates canary testing for KServe inference services where model metrics are already being written to Prometheus. We will use [Iter8](https://iter8.tools) to read the metrics from Prometheus and valide service-level objectives (SLOs) for two versions of an inference service. 

[Iter8](https://iter8.tools) is an open-source Kubernetes release optimizer that makes it easy to ensure that your ML models perform well and maximize business value.

![Canary test](grpc.png)

## Setup

Install Prometheus monitoring for KServe [using these instructions](https://github.com/kserve/kserve/tree/master/docs/samples/metrics-and-monitoring#install-prometheus).

## Deploy an InferenceService

Create an InferenceService that exposes an gRPC port. The following serves the SciKit [iris model](https://kserve.github.io/website/0.10/modelserving/v1beta1/rollout/canary-example/):

```shell
kubectl apply -f - <<EOF
apiVersion: "serving.kserve.io/v1beta1"
kind: "InferenceService"
metadata:
  name: "sklearn-iris"
spec:
  predictor:
    model:
      modelFormat:
        name: sklearn
      storageUri: "gs://kfserving-examples/models/sklearn/1.0/model"
EOF
```

Update the inference service with a canary model, `model-2`, configured to receive 10% of prediction requests.

```shell
kubectl apply -f - <<EOF
apiVersion: "serving.kserve.io/v1beta1"
kind: "InferenceService"
metadata:
  name: "sklearn-iris"
spec:
  predictor:
    canaryTrafficPercent: 10
    model:
      modelFormat:
        name: sklearn
      storageUri: "gs://kfserving-examples/models/sklearn/1.0/model-2"
EOF
```

## Install Iter8 CLI
Install the Iter8 CLI using `brew` as follows. You can also install using pre-built binaries as described [here](https://iter8.tools/0.13/getting-started/install/).

```shell
brew tap iter8-tools/iter8
brew install iter8@0.13
```

## Generate Load

In a production cluster with real users sending prediction requests, there would be no need to generate load. However, you can generate load as follows. First, port-forward local requests to the ingress gateway.

```shell
INGRESS_GATEWAY=$(kubectl get svc --namespace istio-system --selector="app=istio-ingressgateway" --output jsonpath='{.items[0].metadata.name}')
kubectl port-forward --namespace istio-system svc/$INGRESS_GATEWAY 8080:80
```

Then send prediction requests to the inference service. The following script generates about one request a second.

```shell
while true; do 
  curl -H 'Host: sklearn-iris.default.example.com' \
    http://localhost:8080/v1/models/sklearn-iris:predict \
    -d '{"instances": [[6.8,  2.8,  4.8,  1.4], [6.0,  3.4,  4.5,  1.6]]}'
  sleep 1
done
```

## Launch an Iter8 experiment
Iter8 introduces the notion of an *experiment* that makes it easy to verify that your inference service is ready, collect latency and error related metrics, and assess SLOs for performance validation. Launch the Iter8 experiment inside the Kubernetes cluster.

```shell
iter8 k launch \
--set "tasks={ready,custommetrics,assess}" \
--set ready.isvc=sklearn-iris \
--set ready.timeout=180s \
--set custommetrics.templates.kserve-prometheus="https://gist.githubusercontent.com/kalantar/adc6c9b0efe483c00b8f0c20605ac36c/raw/c4562e87b7ac0652b0e46f8f494d024307bff7a1/kserve-prometheus.tpl" \
--set custommetrics.values.labels.service_name=sklearn-iris-predictor-default \
--set 'custommetrics.versionValues[0].labels.revision_name=sklearn-iris-predictor-default-00002' \
--set 'custommetrics.versionValues[1].labels.revision_name=sklearn-iris-predictor-default-00001' \
--set "custommetrics.values.latencyPercentiles={50,75,90,95}" \
--set assess.SLOs.upper.kserve-prometheus/latency-mean=50 \
--set assess.SLOs.upper.kserve-prometheus/latency-p90=75 \
--set assess.SLOs.upper.kserve-prometheus/error-count=0 \
--set runner=cronjob \
--set cronjobSchedule="*/1 * * * *"
```

### More about this Iter8 experiment

1. This experiment consists of three [tasks](https://iter8.tools/0.13/getting-started/concepts/#iter8-experiment), namely, [ready](https://iter8.tools/0.13/user-guide/tasks/ready), [custommetrics](https://iter8.tools/0.13/user-guide/tasks/custommetrics), and [assess](https://iter8.tools/0.13/user-guide/tasks/assess).

    * The [ready](https://iter8.tools/0.13/user-guide/tasks/ready) task checks if the `sklearn-iris` inference service exists and is ready to serve user requests.

    * The [custommetrics](https://iter8.tools/0.13/user-guide/tasks/custommetrics) task collect errors and response time metrics from Prometheus.

    * The [assess](https://iter8.tools/0.13/user-guide/tasks/assess) task verifies if the app satisfies the specified SLOs: i) the mean latency of the service does not exceed 50 msec, ii) the 90th percentile latency of the service does not exceed 75 msec, and iii) there are no errors (4xx or 5xx response codes) in the responses.

2. This is a [multi-loop experiment](https://iter8.tools/0.13/getting-started/concepts/#iter8-experiment) where all the previously mentioned tasks will run repeatedly per the `cronjobSchedule`. Hence, its [runner](https://iter8.tools/0.13/getting-started/concepts/#how-it-works) value is set to `cronjob`.

## View experiment report
```shell
iter8 k report -o html > report.html # view in a browser
```

Below is a sample HTML report.

![HTML report](report.png)

You can also [assert various conditions](https://iter8.tools/0.13/getting-started/your-first-experiment/#assert-experiment-outcomes) about the outcome of the experiment and [view the execution logs](https://iter8.tools/0.13/getting-started/your-first-experiment/#view-experiment-logs) for the experiment.

## Cleanup
Delete the Iter8 experiment and KServe inference service.

```shell
iter8 k delete
kubectl delete isvc sklearn-iris
```
