# KServe Python SDK
Python SDK for KServe Server and Client.

## Installation

KServe Python SDK can be installed by `pip` or `Setuptools`.

### pip install

```sh
pip install kserve
```

### Setuptools

Install via [Setuptools](http://pypi.python.org/pypi/setuptools).

```sh
python setup.py install --user
```
(or `sudo python setup.py install` to install the package for all users)


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

## KServe Client

### Getting Started

KServe's python client interacts with KServe control plane APIs for executing operations on a remote KServe cluster, such as creating, patching and deleting of a InferenceService instance. See the [Sample for Python SDK Client](../../docs/samples/client) to get started.

### Documentation for Client API

Please review [KServe Client API](https://github.com/kserve/website/blob/main/docs/sdk_docs/docs/KServeClient.md) docs.

## Documentation For Models

 - [KnativeAddressable](docs/KnativeAddressable.md)
 - [KnativeCondition](docs/KnativeCondition.md)
 - [KnativeURL](docs/KnativeURL.md)
 - [KnativeVolatileTime](docs/KnativeVolatileTime.md)
 - [NetUrlUserinfo](docs/NetUrlUserinfo.md)
 - [V1beta1AIXExplainerSpec](docs/V1beta1AIXExplainerSpec.md)
 - [V1beta1AlibiExplainerSpec](docs/V1beta1AlibiExplainerSpec.md)
 - [V1beta1Batcher](docs/V1beta1Batcher.md)
 - [V1beta1ComponentExtensionSpec](docs/V1beta1ComponentExtensionSpec.md)
 - [V1beta1ComponentStatusSpec](docs/V1beta1ComponentStatusSpec.md)
 - [V1beta1CustomExplainer](docs/V1beta1CustomExplainer.md)
 - [V1beta1CustomPredictor](docs/V1beta1CustomPredictor.md)
 - [V1beta1CustomTransformer](docs/V1beta1CustomTransformer.md)
 - [V1beta1ExplainerConfig](docs/V1beta1ExplainerConfig.md)
 - [V1beta1ExplainerSpec](docs/V1beta1ExplainerSpec.md)
 - [V1beta1ExplainersConfig](docs/V1beta1ExplainersConfig.md)
 - [V1beta1InferenceService](docs/V1beta1InferenceService.md)
 - [V1beta1InferenceServiceList](docs/V1beta1InferenceServiceList.md)
 - [V1beta1InferenceServiceSpec](docs/V1beta1InferenceServiceSpec.md)
 - [V1beta1InferenceServiceStatus](docs/V1beta1InferenceServiceStatus.md)
 - [V1beta1InferenceServicesConfig](docs/V1beta1InferenceServicesConfig.md)
 - [V1beta1IngressConfig](docs/V1beta1IngressConfig.md)
 - [V1beta1LoggerSpec](docs/V1beta1LoggerSpec.md)
 - [V1beta1ModelSpec](docs/V1beta1ModelSpec.md)
 - [V1beta1ONNXRuntimeSpec](docs/V1beta1ONNXRuntimeSpec.md)
 - [V1beta1PodSpec](docs/V1beta1PodSpec.md)
 - [V1beta1PredictorConfig](docs/V1beta1PredictorConfig.md)
 - [V1beta1PredictorExtensionSpec](docs/V1beta1PredictorExtensionSpec.md)
 - [V1beta1PredictorSpec](docs/V1beta1PredictorSpec.md)
 - [V1beta1PredictorsConfig](docs/V1beta1PredictorsConfig.md)
 - [V1beta1SKLearnSpec](docs/V1beta1SKLearnSpec.md)
 - [V1beta1TFServingSpec](docs/V1beta1TFServingSpec.md)
 - [V1beta1TorchServeSpec](docs/V1beta1TorchServeSpec.md)
 - [V1beta1TrainedModel](docs/V1beta1TrainedModel.md)
 - [V1beta1TrainedModelList](docs/V1beta1TrainedModelList.md)
 - [V1beta1TrainedModelSpec](docs/V1beta1TrainedModelSpec.md)
 - [V1beta1TrainedModelStatus](docs/V1beta1TrainedModelStatus.md)
 - [V1beta1TransformerConfig](docs/V1beta1TransformerConfig.md)
 - [V1beta1TransformerSpec](docs/V1beta1TransformerSpec.md)
 - [V1beta1TransformersConfig](docs/V1beta1TransformersConfig.md)
 - [V1beta1TritonSpec](docs/V1beta1TritonSpec.md)
 - [V1beta1XGBoostSpec](docs/V1beta1XGBoostSpec.md)
