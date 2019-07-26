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
KFServing is a unit of model serving. KFServing's python libraries implement a standardized KFServing library that is extended by model serving frameworks such as XGBoost and PyTorch. It encapsulates data plane API definitions and storage retrieval for models.

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

See the [Sample for KFServing Python SDK](../../docs/samples/client/kfserving_sdk_sample.ipynb) to get started.

### Documentation for Client API

Class | Method |  Description
------------ | ------------- | -------------
[KFServingClient](docs/KFServingClient.md) | [create](docs/KFServingClient.md#create) | Create the provided KFService in the specified namespace|
[KFServingClient](docs/KFServingClient.md) | [get](docs/KFServingClient.md#get)    | Get the created KFService in the specified namespace|
[KFServingClient](docs/KFServingClient.md) | [patch](docs/KFServingClient.md#patch)   | Patch the created KFService in the specified namespace |
[KFServingClient](docs/KFServingClient.md) | [delete](docs/KFServingClient.md#delete) | Delete the created KFService in the specified namespace |


### Documentation For Client Models

 - [KnativeCondition](docs/KnativeCondition.md)
 - [KnativeVolatileTime](docs/KnativeVolatileTime.md)
 - [V1alpha1CustomSpec](docs/V1alpha1CustomSpec.md)
 - [V1alpha1FrameworkConfig](docs/V1alpha1FrameworkConfig.md)
 - [V1alpha1FrameworksConfig](docs/V1alpha1FrameworksConfig.md)
 - [V1alpha1KFService](docs/V1alpha1KFService.md)
 - [V1alpha1KFServiceList](docs/V1alpha1KFServiceList.md)
 - [V1alpha1KFServiceSpec](docs/V1alpha1KFServiceSpec.md)
 - [V1alpha1KFServiceStatus](docs/V1alpha1KFServiceStatus.md)
 - [V1alpha1ModelSpec](docs/V1alpha1ModelSpec.md)
 - [V1alpha1PyTorchSpec](docs/V1alpha1PyTorchSpec.md)
 - [V1alpha1SKLearnSpec](docs/V1alpha1SKLearnSpec.md)
 - [V1alpha1StatusConfigurationSpec](docs/V1alpha1StatusConfigurationSpec.md)
 - [V1alpha1TensorRTSpec](docs/V1alpha1TensorRTSpec.md)
 - [V1alpha1TensorflowSpec](docs/V1alpha1TensorflowSpec.md)
 - [V1alpha1XGBoostSpec](docs/V1alpha1XGBoostSpec.md)
