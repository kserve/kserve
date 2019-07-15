# KFServingApi

Class | Method |  Description
------------ | ------------- | -------------
KFServingApi | [deploy](#deploy) | Create the provided KFService in the specified namespace|
KFServingApi | [get](#get)    | Get the deployed KFService in the specified namespace|
KFServingApi | [patch](#patch)  | Patch the deployed KFService in the specified namespace |
KFServingApi | [delete](#delete) | Delete the deployed KFService in the specified namespace |

## deploy
> deploy(kfservice, namespace='default')

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
                          metadata=client.V1ObjectMeta(name='flower-sample'),
                          spec=V1alpha1KFServiceSpec(default=default_model_spec))


KFServing = KFServingApi()
KFServing.deploy(kfsvc, namespace='kubeflow')
```

### Parameters
Name | Type |  Description | Notes
------------ | ------------- | ------------- | -------------
kfservice  | [V1alpha1KFService](docs/V1alpha1KFService.md) | kfservice defination| |
namespace | str | object name and auth scope, such as for teams and projects | |

### Return type
object

## get
> get(name, namespace='default')

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
namespace | str | object name and auth scope, such as for teams and projects | |

### Return type
object


## patch
> patch(name, kfservice, namespace='default')

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
                          metadata=client.V1ObjectMeta(name='flower-sample'),
                          spec=V1alpha1KFServiceSpec(default=default_model_spec,
                                                     canary=canary_model_spec,
                                                     canary_traffic_percent=10))

KFServing = KFServingApi()
KFServing.patch(kfsvc, namespace='kubeflow')
```

### Parameters
Name | Type |  Description | Notes
------------ | ------------- | ------------- | -------------
kfservice  | [V1alpha1KFService](docs/V1alpha1KFService.md) | kfservice defination| |
namespace | str | object name and auth scope, such as for teams and projects | |

### Return type
object


## delete
> delete(name, namespace='default')

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
namespace | str | object name and auth scope, such as for teams and projects | |

### Return type
object
