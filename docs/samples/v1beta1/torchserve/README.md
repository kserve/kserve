# Predict on a InferenceService using TorchServe

In this example, we use a trained PyTorch MNIST model to predict handwritten digits by running an inference service with [TorchServe](https://github.com/pytorch/serve) predictor.

## Setup

1. Your ~/.kube/config should point to a cluster with [KServe installed](https://github.com/kserve/kserve#installation).
2. Your cluster's Istio Ingress gateway must be [network accessible](https://istio.io/latest/docs/tasks/traffic-management/ingress/ingress-control/).

## Creating Model Storage with a Model Archive File

TorchServe provides a utility to package all the model artifacts into a single [Torchserve Model Archive Files (MAR)](https://github.com/pytorch/serve/blob/master/model-archiver/README.md).

You can store your model and dependent files on remote storage or local persistent volume, the MNIST model and dependent files can be obtained
from [here](https://github.com/pytorch/serve/tree/master/examples/image_classifier/mnist).

The KServe/TorchServe integration expects following model store layout.

```bash
├── config
│   ├── config.properties
├── model-store
│   ├── densenet_161.mar
│   ├── mnist.mar
```

> **Note**: 
> For remote storage you can choose to start the example using the prebuilt MNIST MAR file stored on KServe example GCS bucket `gs://kfserving-examples/models/torchserve/image_classifier/v1`, or generate the MAR file with `torch-model-archiver` and create the model store on remote storage according to the above layout.
>   ```bash
>    torch-model-archiver --model-name mnist --version 1.0 \
>    --model-file model-archiver/model-store/mnist/mnist.py \
>    --serialized-file model-archiver/model-store/mnist/mnist_cnn.pt \
>    --handler model-archiver/model-store/mnist/mnist_handler.py \
>    ```
>    For PVC user please refer to [model archive file generation](./model-archiver/README.md) for auto generation of MAR files with the model and dependent files.

The requests are converted from KServe inference request format to torchserve request format and sent to the `inference_address` configured via local socket.

**Warning**: The `service_envelope` property has beed dreprecated and replaced with `enable_envvars_config` set to true. This enables the service envelope to be set on runtime.

## TorchServe with KFS Envelope Inference Endpoints

The KServe/TorchServe integration supports both KServe v1 and v2 protocols.

V1 Protocol

| API  | Verb | Path | Payload |
| ------------- | ------------- | ------------- | ------------- |
| Predict  | POST  | /v1/models/<model_name>:predict  | Request:{"instances": []}  Response:{"predictions": []} |
| Explain  | POST  | /v1/models/<model_name>:explain  | Request:{"instances": []}  Response:{"predictions": [], "explanations": []}   ||

V2 Protocol

| API  | Verb | Path | Payload |
| ------------- | ------------- | ------------- | ------------- |
| Predict  | POST  | /v2/models/<model_name>/infer  | Request: {"id" : $string #optional,"parameters" : $parameters #optional,"inputs" : [ $request_input, ... ],"outputs" : [ $request_output, ... ] #optional}  Response:{"model_name" : $string,"model_version" : $string #optional,"id" : $string,"parameters" : $parameters #optional,"outputs" : [ $response_output, ... ]}
| Explain  | POST  | /v2/models/<model_name>/explain  | Request: {"id" : $string #optional,"parameters" : $parameters #optional,"inputs" : [ $request_input, ... ],"outputs" : [ $request_output, ... ] #optional}  Response:{"model_name" : $string,"model_version" : $string #optional,"id" : $string,"parameters" : $parameters #optional,"outputs" : [ $response_output, ... ]} ||

[Sample Requests for Text and Image Classification](https://github.com/pytorch/serve/tree/master/kubernetes/kserve/kf_request_json)

## Autoscaling

One of the main serverless inference features is to automatically scale the replicas of an `InferenceService` matching the incoming workload.
KServe by default enables [Knative Pod Autoscaler](https://knative.dev/docs/serving/autoscaling/) which watches traffic flow and scales up and down based on the configured metrics.

[Autoscaling Example](autoscaling/README.md)

## Canary Rollout

Canary rollout is a deployment strategy when you release a new version of model to a small percent of the production traffic.

[Canary Deployment](canary/README.md)

## Monitoring

[Metrics Exposure and Grafana Dashboard Setup](metrics/README.md)
