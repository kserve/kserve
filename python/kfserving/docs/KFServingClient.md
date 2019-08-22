# KFServingClient

> KFServingClient(config_file=None, context=None, client_configuration=None, persist_config=True)

User can loads authentication and cluster information from kube-config file and stores them in kubernetes.client.configuration. Parameters are as following:

parameter |  Description
------------ | -------------
config_file | Name of the kube-config file. Defaults to `~/.kube/config`. |
context |Set the active context. If is set to None, current_context from config file will be used.|
client_configuration | The kubernetes.client.Configuration to set configs to.|
persist_config | If True, config file will be updated when changed (e.g GCP token refresh).|


The APIs for KFServingClient are as following:

Class | Method |  Description
------------ | ------------- | -------------
KFServingClient | [create](#create) | Create KFService|
KFServingClient | [get](#get)    | Get the specified KFService|
KFServingClient | [patch](#patch)  | Patch the specified KFService|
KFServingClient | [delete](#delete) | Delete the specified KFService |

## create
> create(kfservice, namespace=None)

Create the provided KFService in the specified namespace

### Example

```python
from kubernetes import client

from kfserving import KFServingClient
from kfserving import constants
from kfserving import V1alpha2ModelSpec
from kfserving import V1alpha2TensorflowSpec
from kfserving import V1alpha2KFServiceSpec
from kfserving import V1alpha2KFService


default_model_spec = V1alpha2ModelSpec(tensorflow=V1alpha2TensorflowSpec(
    model_uri='gs://kfserving-samples/models/tensorflow/flowers'))

kfsvc = V1alpha2KFService(api_version=constants.KFSERVING_GROUP + '/' + constants.KFSERVING_VERSION,
                          kind=constants.KFSERVING_KIND,
                          metadata=client.V1ObjectMeta(name='flower-sample', namespace='kubeflow'),
                          spec=V1alpha2KFServiceSpec(default=default_model_spec))


KFServing = KFServingClient()
KFServing.create(kfsvc)
```

### Parameters
Name | Type |  Description | Notes
------------ | ------------- | ------------- | -------------
kfservice  | [V1alpha2KFService](V1alpha2KFService.md) | kfservice defination| |
namespace | str | Namespace for kfservice deploying to. If the `namespace` is not defined, will align with kfservice definition, or use current or default namespace if namespace is not specified in kfservice definition.  | |

### Return type
object

## get
> get(name, namespace=None)

Get the created KFService in the specified namespace

### Example

```python
from kfserving import KFServingClient

KFServing = KFServingClient()
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

Patch the created KFService in the specified namespace

### Example

```python
from kubernetes import client

from kfserving import V1alpha2ModelSpec
from kfserving import V1alpha2TensorflowSpec
from kfserving import V1alpha2KFServiceSpec
from kfserving import V1alpha2KFService
from kfserving import KFServingClient

default_model_spec = V1alpha2ModelSpec(tensorflow=V1alpha2TensorflowSpec(
    model_uri='gs://kfserving-samples/models/tensorflow/flowers'))
canary_model_spec = V1alpha2ModelSpec(tensorflow=V1alpha2TensorflowSpec(
    model_uri='gs://kfserving-samples/models/tensorflow/flowers'))

kfsvc = V1alpha2KFService(api_version=constants.KFSERVING_GROUP + '/' + constants.KFSERVING_VERSION,
                          kind=constants.KFSERVING_KIND,
                          metadata=client.V1ObjectMeta(name='flower-sample', namespace='kubeflow'),
                          spec=V1alpha2KFServiceSpec(default=default_model_spec,
                                                     canary=canary_model_spec,
                                                     canary_traffic_percent=10))

KFServing = KFServingClient()
KFServing.patch('flower-sample', kfsvc)
```

### Parameters
Name | Type |  Description | Notes
------------ | ------------- | ------------- | -------------
kfservice  | [V1alpha2KFService](V1alpha2KFService.md) | kfservice defination| |
namespace | str | The kfservice's namespace for patching. If the `namespace` is not defined, will align with kfservice definition, or use current or default namespace if namespace is not specified in kfservice definition. | |

### Return type
object


## delete
> delete(name, namespace=None)

Delete the created KFService in the specified namespace

### Example

```python
from kfserving import KFServingClient

KFServing = KFServingClient()
KFServing.get('flower-sample', namespace='kubeflow')
```

### Parameters
Name | Type |  Description | Notes
------------ | ------------- | ------------- | -------------
Name  | str | kfservice name| |
namespace | str | The kfservice's namespace. Defaults to current or default namespace. | |

### Return type
object
