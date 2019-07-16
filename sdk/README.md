# kfserving
Python SDK for KFServing

## Requirements.

Python 3.5+

## Installation & Usage
### pip install

```sh
pip install kfserving
```

Then import the package:
```python
import kfserving 
```

### Setuptools

Install via [Setuptools](http://pypi.python.org/pypi/setuptools).

```sh
python setup.py install --user
```
(or `sudo python setup.py install` to install the package for all users)

Then import the package:
```python
import kfserving
```

## Getting Started

See the [Sample for KFServing Python SDK](sample/kfserving_sdk_sample.ipynb) to get started.

## Documentation for API Endpoints

Class | Method |  Description
------------ | ------------- | -------------
[KFServingClient](docs/KFServingClient.md) | [create](docs/KFServingClient.md#create) | Create the provided KFService in the specified namespace|
[KFServingClient](docs/KFServingClient.md) | [get](docs/KFServingClient.md#get)    | Get the created KFService in the specified namespace|
[KFServingClient](docs/KFServingClient.md) | [patch](docs/KFServingClient.md#patch)   | Patch the created KFService in the specified namespace |
[KFServingClient](docs/KFServingClient.md) | [delete](docs/KFServingClient.md#delete) | Delete the created KFService in the specified namespace |


## Documentation For Models

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


## Documentation For Authorization

 All endpoints do not require authorization.


## Author


