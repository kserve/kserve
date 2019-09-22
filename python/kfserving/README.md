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
* Metrics Handler

KFServing supports the following storage providers:

* Google Cloud Storage with a prefix: "gs://"
    * By default, it uses `GOOGLE_APPLICATION_CREDENTIALS` environment variable for user authentication.
    * If `GOOGLE_APPLICATION_CREDENTIALS` is not provided, anonymous client will be used to download the artifacts.
* S3 Compatible Object Storage with a prefix "s3://"
    * By default, it uses `S3_ENDPOINT`, `AWS_ACCESS_KEY_ID`, and `AWS_SECRET_ACCESS_KEY` environment variables for user authentication.
* Azure Blob Storage with the format: "https://{$STORAGE_ACCOUNT_NAME}.blob.core.windows.net/{$CONTAINER}/{$PATH}"
    * By default, it uses anonymous client to download the artifacts.
    * For e.g. https://kfserving.blob.core.windows.net/tensorrt/simple_string/
* Local filesystem either without any prefix or with a prefix "file://". For example:
    * Absolute path: `/absolute/path` or `file:///absolute/path`
    * Relative path: `relative/path` or `file://relative/path`
    * For local filesystem, we recommended to use relative path without any prefix.

## KFServing Client

### Getting Started

KFServing's python client interacts with KFServing APIs for executing operations on a remote KFServing cluster, such as creating, patching and deleting of a KFService instance. See the [Sample for KFServing Python SDK Client](../../docs/samples/client/kfserving_sdk_sample.ipynb) to get started.

### Documentation for Client API

Class | Method |  Description
------------ | ------------- | -------------
[KFServingClient](docs/KFServingClient.md) | [create](docs/KFServingClient.md#create) | Create the provided KFService in the specified namespace|
[KFServingClient](docs/KFServingClient.md) | [get](docs/KFServingClient.md#get)    | Get the created KFService in the specified namespace|
[KFServingClient](docs/KFServingClient.md) | [patch](docs/KFServingClient.md#patch)   | Patch the created KFService in the specified namespace |
[KFServingClient](docs/KFServingClient.md) | [delete](docs/KFServingClient.md#delete) | Delete the created KFService in the specified namespace |


## Documentation For Models

 - [KnativeCondition](docs/KnativeCondition.md)
 - [KnativeVolatileTime](docs/KnativeVolatileTime.md)
 - [V1alpha2AlibiExplainerSpec](docs/V1alpha2AlibiExplainerSpec.md)
 - [V1alpha2CustomSpec](docs/V1alpha2CustomSpec.md)
 - [V1alpha2DeploymentSpec](docs/V1alpha2DeploymentSpec.md)
 - [V1alpha2EndpointSpec](docs/V1alpha2EndpointSpec.md)
 - [V1alpha2ExplainerSpec](docs/V1alpha2ExplainerSpec.md)
 - [V1alpha2FrameworkConfig](docs/V1alpha2FrameworkConfig.md)
 - [V1alpha2FrameworksConfig](docs/V1alpha2FrameworksConfig.md)
 - [V1alpha2KFService](docs/V1alpha2KFService.md)
 - [V1alpha2KFServiceList](docs/V1alpha2KFServiceList.md)
 - [V1alpha2KFServiceSpec](docs/V1alpha2KFServiceSpec.md)
 - [V1alpha2KFServiceStatus](docs/V1alpha2KFServiceStatus.md)
 - [V1alpha2ONNXSpec](docs/V1alpha2ONNXSpec.md)
 - [V1alpha2PredictorSpec](docs/V1alpha2PredictorSpec.md)
 - [V1alpha2PyTorchSpec](docs/V1alpha2PyTorchSpec.md)
 - [V1alpha2SKLearnSpec](docs/V1alpha2SKLearnSpec.md)
 - [V1alpha2StatusConfigurationSpec](docs/V1alpha2StatusConfigurationSpec.md)
 - [V1alpha2TensorRTSpec](docs/V1alpha2TensorRTSpec.md)
 - [V1alpha2TensorflowSpec](docs/V1alpha2TensorflowSpec.md)
 - [V1alpha2TransformerSpec](docs/V1alpha2TransformerSpec.md)
 - [V1alpha2XGBoostSpec](docs/V1alpha2XGBoostSpec.md)
