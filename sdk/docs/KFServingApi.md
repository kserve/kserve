# KFServingApi

Class | Method |  Description
------------ | ------------- | -------------
KFServingApi | [deploy](#deploy) | Create KFService|
KFServingApi | [get](#get)    | Get the specified KFService|
KFServingApi | [patch](#patch)  | Patch the specified KFService|
KFServingApi | [delete](#delete) | Delete the specified KFService |

## deploy
> deploy(kfservice, namespace=None)

Create the provided KFService in the specified namespace

### Example

```python
from kubernetes import client

from kfserving.api.kf_serving_api import KFServingApi
from kfserving.constants import constants
from kfserving.models.v1alpha1_model_spec import V1alpha1ModelSpec
from kfserving.models.v1alpha1_tensorflow_spec import V1alpha1TensorflowSpec
from kfserving.models.v1alpha1_kf_service_spec import V1alpha1KFServiceSpec
from kfserving.models.v1alpha1_kf_service import V1alpha1KFService


default_model_spec = V1alpha1ModelSpec(tensorflow=V1alpha1TensorflowSpec(
    model_uri='gs://kfserving-samples/models/tensorflow/flowers'))

kfsvc = V1alpha1KFService(api_version=constants.KFSERVING_GROUP + '/' + constants.KFSERVING_VERSION,
                          kind=constants.KFSERVING_KIND,
                          metadata=client.V1ObjectMeta(name='flower-sample', namespace='kubeflow'),
                          spec=V1alpha1KFServiceSpec(default=default_model_spec))


KFServing = KFServingApi()
KFServing.deploy(kfsvc)
```

### Parameters
Name | Type |  Description | Notes
------------ | ------------- | ------------- | -------------
kfservice  | [V1alpha1KFService](docs/V1alpha1KFService.md) | kfservice defination| |
namespace | str | Namespace for kfservice deploying to. If the `namespace` is not defined, will align with kfservice definition, or use current or default namespace if namespace is not specified in kfservice definition.  | |

### Return type
object

## get
> get(name, namespace=None)

Get the deployed KFService in the specified namespace

### Example

```python
from kfserving.api.kf_serving_api import KFServingApi

KFServing = KFServingApi()
KFServing.get('flower-sample', namespace='kubeflow')
```

### Parameters
Name | Type |  Description | Notes
------------ | ------------- | ------------- | -------------
name  | str | kfservice name| |
namespace | str | The kfservice's namespace. Defaults to current or default namespace.| |

### Return type
object


## patch
> patch(name, kfservice, namespace=None)

Patch the deployed KFService in the specified namespace

### Example

```python
from kubernetes import client

from kfserving.models.v1alpha1_model_spec import V1alpha1ModelSpec
from kfserving.models.v1alpha1_tensorflow_spec import V1alpha1TensorflowSpec
from kfserving.models.v1alpha1_kf_service_spec import V1alpha1KFServiceSpec
from kfserving.models.v1alpha1_kf_service import V1alpha1KFService
from kfserving.api.kf_serving_api import KFServingApi

default_model_spec = V1alpha1ModelSpec(tensorflow=V1alpha1TensorflowSpec(
    model_uri='gs://kfserving-samples/models/tensorflow/flowers'))
canary_model_spec = V1alpha1ModelSpec(tensorflow=V1alpha1TensorflowSpec(
    model_uri='gs://kfserving-samples/models/tensorflow/flowers'))

kfsvc = V1alpha1KFService(api_version=constants.KFSERVING_GROUP + '/' + constants.KFSERVING_VERSION,
                          kind=constants.KFSERVING_KIND,
                          metadata=client.V1ObjectMeta(name='flower-sample', namespace='kubeflow'),
                          spec=V1alpha1KFServiceSpec(default=default_model_spec,
                                                     canary=canary_model_spec,
                                                     canary_traffic_percent=10))

KFServing = KFServingApi()
KFServing.patch('flower-sample', kfsvc)
```

### Parameters
Name | Type |  Description | Notes
------------ | ------------- | ------------- | -------------
kfservice  | [V1alpha1KFService](docs/V1alpha1KFService.md) | kfservice defination| |
namespace | str | The kfservice's namespace for patching. If the `namespace` is not defined, will align with kfservice definition, or use current or default namespace if namespace is not specified in kfservice definition. | |

### Return type
object


## delete
> delete(name, namespace=None)

Delete the deployed KFService in the specified namespace

### Example

```python
from kfserving.api.kf_serving_api import KFServingApi

KFServing = KFServingApi()
KFServing.get('flower-sample', namespace='kubeflow')
```

### Parameters
Name | Type |  Description | Notes
------------ | ------------- | ------------- | -------------
Name  | str | kfservice name| |
namespace | str | The kfservice's namespace. Defaults to current or default namespace. | |

### Return type
object
