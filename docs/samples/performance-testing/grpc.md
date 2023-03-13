# gRPC Performance test

This tutorial demonstrates performance testing for KServe inference services using gRPC. We will use [Iter8](https://iter8.tools) for generating load and validing service-level objectives (SLOs) for the inference service. [Iter8](https://iter8.tools) is an open-source Kubernetes release optimizer that makes it easy to ensure that your ML models perform well and maximize business value.

![Iter8 gRPC performanc test](grpc.png)

***

> Performance testing of KServe inference services using HTTP is described [here](README.md). Canary testing using Prometheus metrics is described [here](../canary-testing/README.md). This tutorial focuses on performance testing of KServe inference services with gRPC endpoints. The main steps in this tutorial are:
> 1. [Deploy an InferenceService](#deploy-an-inferenceservice)
> 2. [Launch an Iter8 experiment](#launch-an-iter8-experiment)
> 3. [View experiment report](#view-experiment-report)

***

## Deploy an InferenceService

Create an InferenceService that exposes an gRPC port. The following serves the Scikit-learn [irisv2 model](https://kserve.github.io/website/0.10/modelserving/v1beta1/sklearn/v2/#deploy-with-inferenceservice):

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
      protocolVersion: v2
      storageUri: "gs://seldon-models/sklearn/mms/lr_model"
      ports:
      - containerPort: 9000
        name: h2c
        protocol: TCP
EOF
```

Verify that your inference service is ready.

```shell
kubectl wait --for=condition=Ready --timeout=600s isvc/sklearn-irisv2
```

## Install Iter8 CLI
Install the Iter8 CLI using `brew` as follows. You can also install using pre-built binaries as described [here](https://iter8.tools/0.13/getting-started/install/).

```shell
brew tap iter8-tools/iter8
brew install iter8@0.13
```

## Launch an Iter8 experiment
Iter8 introduces the notion of an *experiment* that makes it easy to verify that your inference service is ready, generate load for the inference, collect latency and error-related metrics, and assess SLOs for performance validation. Launch the Iter8 experiment inside the Kubernetes cluster.

```shell
iter8 k launch \
--set "tasks={ready,grpc,assess}" \
--set ready.isvc=sklearn-irisv2 \
--set ready.timeout=600s \
--set grpc.protoURL=https://raw.githubusercontent.com/kserve/kserve/master/docs/predict-api/v2/grpc_predict_v2.proto \
--set grpc.host=sklearn-irisv2-predictor-default.default.svc.cluster.local:80 \
--set grpc.call=inference.GRPCInferenceService.ModelInfer \
--set grpc.dataURL=https://gist.githubusercontent.com/kalantar/6e9eaa03cad8f4e86b20eeb712efef45/raw/56496ed5fa9078b8c9cdad590d275ab93beaaee4/sklearn-irisv2-input-grpc.json \
--set grpc.warmupNumRequests=10 \
--set assess.SLOs.upper.grpc/latency/mean=500 \
--set assess.SLOs.upper.grpc/latency/p90=900 \
--set assess.SLOs.upper.grpc/error-count=0 \
--set runner=job
```

### More about this Iter8 experiment

1. This experiment consists of three [tasks](https://iter8.tools/0.13/getting-started/concepts/#iter8-experiment), namely, [ready](https://iter8.tools/0.13/user-guide/tasks/ready), [grpc](https://iter8.tools/0.13/user-guide/tasks/grpc), and [assess](https://iter8.tools/0.13/user-guide/tasks/assess).

    * The [ready](https://iter8.tools/0.13/user-guide/tasks/ready) task checks if the `sklearn-irisv2` inference service exists and is ready to serve user requests.

    * The [grpc](https://iter8.tools/0.13/user-guide/tasks/grpc) task sends requests to the cluster-local gRPC service whose endpoint is `sklearn-irisv2-predictor-default.default.svc.cluster.local:80`, and collects [Iter8's built-in gRPC load test metrics](https://iter8.tools/0.13/user-guide/tasks/grpc#metrics). As part of these requests, it uses the JSON data at [this dataURL](https://gist.githubusercontent.com/kalantar/6e9eaa03cad8f4e86b20eeb712efef45/raw/56496ed5fa9078b8c9cdad590d275ab93beaaee4/sklearn-irisv2-input-grpc.json) as the request.

    * The [assess](https://iter8.tools/0.13/user-guide/tasks/assess) task verifies if the app satisfies the specified SLOs: i) the mean latency of the service does not exceed 500 msec, ii) the 90th percentile latency of the service does not exceed 1000 msec, and iii) there are no errors (4xx or 5xx response codes) in the responses.

2. This is a single-loop in which all the previously mentioned tasks will run once and the experiment will finish.

## View experiment report
```shell
iter8 k report -o html > report.html # view in a browser
```

Below is a sample HTML report.

![gRPC report](grpc-report.png)

You can also [assert various conditions](https://iter8.tools/0.13/getting-started/your-first-experiment/#assert-experiment-outcomes) about the outcome of the experiment and [view the execution logs](https://iter8.tools/0.13/getting-started/your-first-experiment/#view-experiment-logs) for the experiment.

## Cleanup
Delete the Iter8 experiment and KServe inference service.

```shell
iter8 k delete
kubectl delete isvc sklearn-irisv2
```

***

This tutorial just scratches the surface of Iter8 experimentation capabilities. For more features (for example, automatically sending [a notification](https://iter8.tools/0.13/user-guide/tasks/slack/#if-parameter) to Slack or GitHub with experiment results), please see [Iter8 documentation](https://iter8.tools).
