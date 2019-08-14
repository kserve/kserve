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
KFServingClient | [create_credentials](#create_credentials) | Create Credentials|
KFServingClient | [create](#create) | Create KFService|
KFServingClient | [get](#get)    | Get the specified KFService|
KFServingClient | [patch](#patch)  | Patch the specified KFService|
KFServingClient | [delete](#delete) | Delete the specified KFService |

## create_credentials
> create_credentials(storage_type, namespace=None, **kwargs)

Create or update `Secret` and `Service Account` for GCS and S3 according to credentials information. Once the `Service Account` created or updated, user can configure the `Service Account` in the [V1alpha1ModelSpec](V1alpha1ModelSpec.md) for kfserving.

The API returns name of the `Service Account`.

### Example

Example for creating GCP credentials.
```python
from kfserving import KFServingClient

KFServing = KFServingClient()
KFServing.create_credentials(storage_type='GCS',
                             namespace='kubeflow',
                             credentials_file='/tmp/gcp.json')
```

Example for creating AWS credentials.
```python
from kfserving import KFServingClient

KFServing = KFServingClient()
KFServing.create_credentials(storage_type='S3',
                             namespace='kubeflow',
                             credentials_file='/tmp/aws/credentials',
                             s3_profile='default',
                             s3_endpoint='s3.us-west-2.amazonaws.com',
                             s3_region='us-west-2',
                             s3_use_https='1',
                             s3_verify_ssl='0')
```

The created `Secret` and `Service Account` will be shown as following:
```
INFO:kfserving.api.set_credentials:Created Secret: kfserving-secret-6tv6l in namespace kubeflow
INFO:kfserving.api.set_credentials:Created Service account: kfserving-sa-tj444 in namespace kubeflow
```

The `create_credentials` also supports specifying an existing service account, if so the API only creates the secret according to credentials information, and attach the created secret with the service account. For example:
```
from kfserving import KFServingClient

KFServing = KFServingClient()
KFServing.create_credentials(storage_type='S3',
                             namespace='kubeflow',
                             credentials_file='/tmp/aws/credentials',
                             service_account = 'kfserving-sa-5q9qc')
``` 

The outputs will be as following:
```
INFO:kfserving.api.create_credentials:Created Secret: kfserving-secret-gxckg in namespace kubeflow
INFO:kfserving.api.create_credentials:Pacthed Service account: kfserving-sa-5q9qc in namespace kubeflow
```

### Parameters
Name | Type | Storage Type | Description
------------ | ------------- | ------------- | -------------
storage_type | str | All |Required. Valid values: GCS or S3 |
namespace | str | All |Optional. The kubenertes namespace. Defaults to current or default namespace.|
credentials_file | str | All |Optional. The path for the GCS or S3 credentials file. The default file for GCS is `~/.config/gcloud/application_default_credentials.json`, and default file for S3 is `~/.aws/credentials`. |
service_account  | str | All |Optional. The name of service account. If the service_account is specified, will attach created secret with the service account, otherwise will create new one and attach with created secret.|
s3_endpoint  | str | S3 only |Optional. The S3 endpoint. |
s3_region  | str | S3 only|Optional. The S3 region By default, regional endpoint is used for S3.| |
s3_use_https  | str | S3 only |Optional. HTTPS is used to access S3 by default, unless `s3_use_https=0` |
s3_verify_ssl  | str | S3 only|Optional. If HTTPS is used, SSL verification could be disabled with `s3_verify_ssl=0` |


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
