# KServe Python SDK
Python SDK for KServe Server and Client.

## Installation

KServe Python SDK can be installed by `pip` or `poetry`.

### pip install

```sh
pip install kserve
```

To install Kserve with storage support
```sh
pip install kserve[storage]
```

### Poetry

Install via [Poetry](https://python-poetry.org/).

```sh
make dev_install
```
To install Kserve with storage support
```sh
poetry install -E storage
```
or 
```sh
poetry install --extras "storage"
```

## KServe Python Server
KServe's python server libraries implement a standardized library that is extended by model serving frameworks such as Scikit Learn, XGBoost and PyTorch. It encapsulates data plane API definitions and storage retrieval for models.

It provides many functionalities, including among others:

* Registering a model and starting the server
* Prediction Handler
* Pre/Post Processing Handler
* Liveness Handler
* Readiness Handlers

It supports the following storage providers:

* Google Cloud Storage with a prefix: "gs://"
    * By default, it uses `GOOGLE_APPLICATION_CREDENTIALS` environment variable for user authentication.
    * If `GOOGLE_APPLICATION_CREDENTIALS` is not provided, anonymous client will be used to download the artifacts.
* S3 Compatible Object Storage with a prefix "s3://"
    * By default, it uses `S3_ENDPOINT`, `AWS_ACCESS_KEY_ID`, and `AWS_SECRET_ACCESS_KEY` environment variables for user authentication.
* Azure Blob Storage with the format: "https://{$STORAGE_ACCOUNT_NAME}.blob.core.windows.net/{$CONTAINER}/{$PATH}"
    * By default, it uses anonymous client to download the artifacts.
    * For e.g. https://kfserving.blob.core.windows.net/triton/simple_string/
* Local filesystem either without any prefix or with a prefix "file://". For example:
    * Absolute path: `/absolute/path` or `file:///absolute/path`
    * Relative path: `relative/path` or `file://relative/path`
    * For local filesystem, we recommended to use relative path without any prefix.
* Persistent Volume Claim (PVC) with the format "pvc://{$pvcname}/[path]".
    * The `pvcname` is the name of the PVC that contains the model.
    * The `[path]` is the relative path to the model on the PVC.
    * For e.g. `pvc://mypvcname/model/path/on/pvc`
* Generic URI, over either `HTTP`, prefixed with `http://` or `HTTPS`, prefixed with `https://`. For example:
    * `https://<some_url>.com/model.joblib`
    * `http://<some_url>.com/model.joblib`

### Metrics

For latency metrics, send a request to `/metrics`. Prometheus latency histograms are emitted for each of the steps (pre/postprocessing, explain, predict).
Additionally, the latencies of each step are logged per request.

| Metric Name                       | Description                    | Type      |
|-----------------------------------|--------------------------------|-----------| 
| request_preprocess_seconds        | pre-processing request latency | Histogram | 
| request_explain_seconds | explain request latency        | Histogram | 
| request_predict_seconds | prediction request latency     | Histogram |
| request_postprocess_seconds    | pre-processing request latency | Histogram | 


## KServe Client

### Getting Started

KServe's python client interacts with KServe control plane APIs for executing operations on a remote KServe cluster, such as creating, patching and deleting of a InferenceService instance. See the [Sample for Python SDK Client](https://github.com/kserve/kserve/tree/master/docs/samples/client) to get started.

### Documentation for Client API

Please review [KServe Client API](https://github.com/kserve/website/blob/main/docs/sdk_docs/docs/KServeClient.md) docs.

## Documentation For Models

 - [KnativeAddressable](https://github.com/kserve/kserve/blob/master/python/kserve/docs/KnativeAddressable.md)
 - [KnativeCondition](https://github.com/kserve/kserve/blob/master/python/kserve/docs/KnativeCondition.md)
 - [KnativeURL](https://github.com/kserve/kserve/blob/master/python/kserve/docs/KnativeURL.md)
 - [KnativeVolatileTime](https://github.com/kserve/kserve/blob/master/python/kserve/docs/KnativeVolatileTime.md)
 - [NetUrlUserinfo](https://github.com/kserve/kserve/blob/master/python/kserve/docs/NetUrlUserinfo.md)
 - [V1alpha1InferenceGraph](https://github.com/kserve/kserve/blob/master/python/kserve/docs/V1alpha1InferenceGraph.md)
 - [V1alpha1InferenceGraphList](https://github.com/kserve/kserve/blob/master/python/kserve/docs/V1alpha1InferenceGraphList.md)
 - [V1alpha1InferenceGraphSpec](https://github.com/kserve/kserve/blob/master/python/kserve/docs/V1alpha1InferenceGraphSpec.md)
 - [V1alpha1InferenceGraphStatus](https://github.com/kserve/kserve/blob/master/python/kserve/docs/V1alpha1InferenceGraphStatus.md)
 - [V1alpha1InferenceRouter](https://github.com/kserve/kserve/blob/master/python/kserve/docs/V1alpha1InferenceRouter.md)
 - [V1alpha1InferenceStep](https://github.com/kserve/kserve/blob/master/python/kserve/docs/V1alpha1InferenceStep.md)
 - [V1alpha1InferenceTarget](https://github.com/kserve/kserve/blob/master/python/kserve/docs/V1alpha1InferenceTarget.md)
 - [V1beta1AlibiExplainerSpec](https://github.com/kserve/kserve/blob/master/python/kserve/docs/V1beta1AlibiExplainerSpec.md)
 - [V1beta1Batcher](https://github.com/kserve/kserve/blob/master/python/kserve/docs/V1beta1Batcher.md)
 - [V1beta1ComponentExtensionSpec](https://github.com/kserve/kserve/blob/master/python/kserve/docs/V1beta1ComponentExtensionSpec.md)
 - [V1beta1ComponentStatusSpec](https://github.com/kserve/kserve/blob/master/python/kserve/docs/V1beta1ComponentStatusSpec.md)
 - [V1beta1CustomExplainer](https://github.com/kserve/kserve/blob/master/python/kserve/docs/V1beta1CustomExplainer.md)
 - [V1beta1CustomPredictor](https://github.com/kserve/kserve/blob/master/python/kserve/docs/V1beta1CustomPredictor.md)
 - [V1beta1CustomTransformer](https://github.com/kserve/kserve/blob/master/python/kserve/docs/V1beta1CustomTransformer.md)
 - [V1beta1ExplainerConfig](https://github.com/kserve/kserve/blob/master/python/kserve/docs/V1beta1ExplainerConfig.md)
 - [V1beta1ExplainerSpec](https://github.com/kserve/kserve/blob/master/python/kserve/docs/V1beta1ExplainerSpec.md)
 - [V1beta1ExplainersConfig](https://github.com/kserve/kserve/blob/master/python/kserve/docs/V1beta1ExplainersConfig.md)
 - [V1beta1InferenceService](https://github.com/kserve/kserve/blob/master/python/kserve/docs/V1beta1InferenceService.md)
 - [V1beta1InferenceServiceList](https://github.com/kserve/kserve/blob/master/python/kserve/docs/V1beta1InferenceServiceList.md)
 - [V1beta1InferenceServiceSpec](https://github.com/kserve/kserve/blob/master/python/kserve/docs/V1beta1InferenceServiceSpec.md)
 - [V1beta1InferenceServiceStatus](https://github.com/kserve/kserve/blob/master/python/kserve/docs/V1beta1InferenceServiceStatus.md)
 - [V1beta1InferenceServicesConfig](https://github.com/kserve/kserve/blob/master/python/kserve/docs/V1beta1InferenceServicesConfig.md)
 - [V1beta1IngressConfig](https://github.com/kserve/kserve/blob/master/python/kserve/docs/V1beta1IngressConfig.md)
 - [V1beta1LoggerSpec](https://github.com/kserve/kserve/blob/master/python/kserve/docs/V1beta1LoggerSpec.md)
 - [V1beta1ModelSpec](https://github.com/kserve/kserve/blob/master/python/kserve/docs/V1beta1ModelSpec.md)
 - [V1beta1ONNXRuntimeSpec](https://github.com/kserve/kserve/blob/master/python/kserve/docs/V1beta1ONNXRuntimeSpec.md)
 - [V1beta1PodSpec](https://github.com/kserve/kserve/blob/master/python/kserve/docs/V1beta1PodSpec.md)
 - [V1beta1PredictorConfig](https://github.com/kserve/kserve/blob/master/python/kserve/docs/V1beta1PredictorConfig.md)
 - [V1beta1PredictorExtensionSpec](https://github.com/kserve/kserve/blob/master/python/kserve/docs/V1beta1PredictorExtensionSpec.md)
 - [V1beta1PredictorSpec](https://github.com/kserve/kserve/blob/master/python/kserve/docs/V1beta1PredictorSpec.md)
 - [V1beta1PredictorsConfig](https://github.com/kserve/kserve/blob/master/python/kserve/docs/V1beta1PredictorsConfig.md)
 - [V1beta1SKLearnSpec](https://github.com/kserve/kserve/blob/master/python/kserve/docs/V1beta1SKLearnSpec.md)
 - [V1beta1TFServingSpec](https://github.com/kserve/kserve/blob/master/python/kserve/docs/V1beta1TFServingSpec.md)
 - [V1beta1TorchServeSpec](https://github.com/kserve/kserve/blob/master/python/kserve/docs/V1beta1TorchServeSpec.md)
 - [V1beta1TrainedModel](https://github.com/kserve/kserve/blob/master/python/kserve/docs/V1beta1TrainedModel.md)
 - [V1beta1TrainedModelList](https://github.com/kserve/kserve/blob/master/python/kserve/docs/V1beta1TrainedModelList.md)
 - [V1beta1TrainedModelSpec](https://github.com/kserve/kserve/blob/master/python/kserve/docs/V1beta1TrainedModelSpec.md)
 - [V1beta1TrainedModelStatus](https://github.com/kserve/kserve/blob/master/python/kserve/docs/V1beta1TrainedModelStatus.md)
 - [V1beta1TransformerConfig](https://github.com/kserve/kserve/blob/master/python/kserve/docs/V1beta1TransformerConfig.md)
 - [V1beta1TransformerSpec](https://github.com/kserve/kserve/blob/master/python/kserve/docs/V1beta1TransformerSpec.md)
 - [V1beta1TransformersConfig](https://github.com/kserve/kserve/blob/master/python/kserve/docs/V1beta1TransformersConfig.md)
 - [V1beta1TritonSpec](https://github.com/kserve/kserve/blob/master/python/kserve/docs/V1beta1TritonSpec.md)
 - [V1beta1XGBoostSpec](https://github.com/kserve/kserve/blob/master/python/kserve/docs/V1beta1XGBoostSpec.md)
