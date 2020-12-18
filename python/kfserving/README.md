# KFServing Python SDK
Python SDK for KFServing Server and Client.

## Installation

KFServing Python SDK can be installed by `pip` or `Setuptools`.

### pip install

```sh
pip install kfserving
```

### Setuptools

Install via [Setuptools](http://pypi.python.org/pypi/setuptools).

```sh
python setup.py install --user
```
(or `sudo python setup.py install` to install the package for all users)


## KFServing Server
KFServing's python server libraries implement a standardized KFServing library that is extended by model serving frameworks such as Scikit Learn, XGBoost and PyTorch. It encapsulates data plane API definitions and storage retrieval for models.

KFServing provides many functionalities, including among others:

* Registering a model and starting the server
* Prediction Handler
* Liveness Handler
* Readiness Handlers

KFServing supports the following storage providers:

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

## KFServing Client

### Getting Started

KFServing's python client interacts with KFServing APIs for executing operations on a remote KFServing cluster, such as creating, patching and deleting of a InferenceService instance. See the [Sample for KFServing Python SDK Client](../../docs/samples/client) to get started.

### Documentation for Client API

Class | Method |  Description
------------ | ------------- | -------------
[KFServingClient](docs/KFServingClient.md) | [set_credentials](docs/KFServingClient.md#set_credentials) | Set Credentials|
[KFServingClient](docs/KFServingClient.md) | [create](docs/KFServingClient.md#create) | Create InferenceService|
[KFServingClient](docs/KFServingClient.md) | [get](docs/KFServingClient.md#get)    | Get or watch the specified InferenceService or all InferenceServices in the namespace |
[KFServingClient](docs/KFServingClient.md) | [patch](docs/KFServingClient.md#patch)  | Patch the specified InferenceService|
[KFServingClient](docs/KFServingClient.md) | [replace](docs/KFServingClient.md#replace) | Replace the specified InferenceService|
[KFServingClient](docs/KFServingClient.md) | [rollout_canary](docs/KFServingClient.md#rollout_canary) | Rollout the traffic on `canary` version for specified InferenceService|
[KFServingClient](docs/KFServingClient.md) | [promote](docs/KFServingClient.md#promote) | Promote the `canary` version of the InferenceService to `default`|
[KFServingClient](docs/KFServingClient.md) | [delete](docs/KFServingClient.md#delete) | Delete the specified InferenceService |
[KFServingClient](docs/KFServingClient.md) | [wait_isvc_ready](docs/KFServingClient.md#wait_isvc_ready) | Wait for the InferenceService to be ready |
[KFServingClient](docs/KFServingClient.md) | [is_isvc_ready](docs/KFServingClient.md#is_isvc_ready) | Check if the InferenceService is ready |

## Documentation For Models

 - [KnativeAddressable](docs/KnativeAddressable.md)
 - [KnativeCondition](docs/KnativeCondition.md)
 - [KnativeURL](docs/KnativeURL.md)
 - [KnativeVolatileTime](docs/KnativeVolatileTime.md)
 - [NetUrlUserinfo](docs/NetUrlUserinfo.md)
 - [V1alpha2AlibiExplainerSpec](docs/V1alpha2AlibiExplainerSpec.md)
 - [V1alpha2Batcher](docs/V1alpha2Batcher.md)
 - [V1alpha2CustomSpec](docs/V1alpha2CustomSpec.md)
 - [V1alpha2DeploymentSpec](docs/V1alpha2DeploymentSpec.md)
 - [V1alpha2EndpointSpec](docs/V1alpha2EndpointSpec.md)
 - [V1alpha2ExplainerSpec](docs/V1alpha2ExplainerSpec.md)
 - [V1alpha2InferenceService](docs/V1alpha2InferenceService.md)
 - [V1alpha2InferenceServiceList](docs/V1alpha2InferenceServiceList.md)
 - [V1alpha2InferenceServiceSpec](docs/V1alpha2InferenceServiceSpec.md)
 - [V1alpha2InferenceServiceStatus](docs/V1alpha2InferenceServiceStatus.md)
 - [V1alpha2Logger](docs/V1alpha2Logger.md)
 - [V1alpha2ONNXSpec](docs/V1alpha2ONNXSpec.md)
 - [V1alpha2PredictorSpec](docs/V1alpha2PredictorSpec.md)
 - [V1alpha2PyTorchSpec](docs/V1alpha2PyTorchSpec.md)
 - [V1alpha2SKLearnSpec](docs/V1alpha2SKLearnSpec.md)
 - [V1alpha2StatusConfigurationSpec](docs/V1alpha2StatusConfigurationSpec.md)
 - [V1alpha2TritonSpec](docs/V1alpha2TritonSpec.md)
 - [V1alpha2TensorflowSpec](docs/V1alpha2TensorflowSpec.md)
 - [V1alpha2TransformerSpec](docs/V1alpha2TransformerSpec.md)
 - [V1alpha2XGBoostSpec](docs/V1alpha2XGBoostSpec.md)
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
