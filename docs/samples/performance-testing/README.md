# HTTP Performance test

This tutorial demonstrates performance testing for KServe inference services usin HTTP. We will use [Iter8](https://iter8.tools) for generating load and validing service-level objectives (SLOs) for the inference service. Performance testing of KServe inference services using gRPC is described [here](grpc.md).

[Iter8](https://iter8.tools) is an open-source Kubernetes release optimizer that makes it easy to ensure that your ML models perform well and maximize business value.

![Iter8 HTTP performanc test](http.png)

***

## Deploy an InferenceService

Create an InferenceService that exposes an HTTP port. The following serves the SciKit [irisv2 model](https://kserve.github.io/website/0.10/modelserving/v1beta1/sklearn/v2/#deploy-with-inferenceservice):

```shell
cat <<EOF | kubectl apply -f -
apiVersion: "serving.kserve.io/v1beta1"
kind: "InferenceService"
metadata:
  name: "sklearn-irisv2"
spec:
  predictor:
    model:
      modelFormat:
        name: sklearn
      runtime: kserve-mlserver
      storageUri: "gs://seldon-models/sklearn/mms/lr_model"
EOF
```

## Install Iter8 CLI
Install the Iter8 CLI using `brew` as follows. You can also install using pre-built binaries as described [here](https://iter8.tools/0.13/getting-started/install/).

```shell
brew tap iter8-tools/iter8
brew install iter8@0.13
```

***

## Launch an Iter8 experiment
Iter8 introduces the notion of an *experiment* that makes it easy to ... . Launch the Iter8 experiment inside the Kubernetes cluster.

```shell
iter8 k launch \
--set "tasks={ready,http,assess}" \
--set ready.isvc=sklearn-irisv2 \
--set ready.timeout=180s \
--set http.url=http://sklearn-irisv2.default.svc.cluster.local/v2/models/sklearn-irisv2/infer \
--set http.payloadURL=https://gist.githubusercontent.com/kalantar/d2dd03e8ebff2c57c3cfa992b44a54ad/raw/97a0480d0dfb1deef56af73a0dd31c80dc9b71f4/sklearn-irisv2-input.json \
--set http.contentType="application/json" \
--set assess.SLOs.upper.http/latency-mean=500 \
--set assess.SLOs.upper.http/error-count=0 \
--set runner=job
```

### More about this Iter8 experiment

1. This experiment consists of three [tasks](concepts.md#iter8-experiment), namely, [ready](../user-guide/tasks/ready.md), [http](../user-guide/tasks/http.md), and [assess](../user-guide/tasks/assess.md). 

    * The [ready](../user-guide/tasks/ready.md) task checks if the `httpbin` deployment exists and is available, and the `httpbin` service exists. 

    * The [http](../user-guide/tasks/http.md) task sends requests to the cluster-local HTTP service whose URL is `http://httpbin.default/get`, and collects [Iter8's built-in HTTP load test metrics](../user-guide/tasks/http.md#metrics). 

    * The [assess](../user-guide/tasks/assess.md) task verifies if the app satisfies the specified SLOs: i) the mean latency of the service does not exceed 50 msec, and ii) there are no errors (4xx or 5xx response codes) in the responses. 

2. This is a [single-loop experiment](concepts.md#iter8-experiment) where all the previously mentioned tasks will run once and the experiment will finish. Hence, its [runner](concepts.md#how-it-works) value is set to `job`.

## View experiment report
--8<-- "docs/getting-started/expreport.md"

You can also assert and view logs as described [here]().

## Cleanup
Remove the Iter8 experiment.

```shell
iter8 k delete
```
