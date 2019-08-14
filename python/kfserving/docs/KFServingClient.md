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
KFServingClient | [create_creds](#create_creds) | Create Credentials|
KFServingClient | [create](#create) | Create KFService|
KFServingClient | [get](#get)    | Get the specified KFService|
KFServingClient | [patch](#patch)  | Patch the specified KFService|
KFServingClient | [delete](#delete) | Delete the specified KFService |

## create_creds
> create_creds(self, platform, namespace=None, **kwargs)

Create `Secret` and `Service Account` for AWS and GCP accordingly to credentials information. Once the `Service Account` ceated, user can configure the `Service Account` in the [V1alpha1ModelSpec](V1alpha1ModelSpec.md) for kfserving.

The API returns name of created `Service Account`.

### Example

Example for creating AWS credentials.
```python
KFServingClient
from kfserving import KFServingClient

KFServing = KFServingClient()
KFServing.create_creds(platform='AWS',
                          namespace='kubeflow',
                          AWS_ACCESS_KEY_ID='MWYyZDFlMmU2N2Rm',
                          AWS_SECRET_ACCESS_KEY='YWRtaW4=',
                          S3_ENDPOINT='s3.us-west-2.amazonaws.com',
                          AWS_REGION='us-west-2',
                          S3_USE_HTTPS='1',
                          S3_VERIFY_SSL='0')
```

Example for creating GCP credentials.
```python
from kfserving import KFServingClient

KFServing = KFServingClient()
KFServing.create_creds(platform='GCP',
                          namespace='kubeflow',
                          GCP_CREDS_FILE='/root/.config/gcloud/application_default_credentials.json')
```
The created Secret and Service Account will be shown as following:
```
INFO:kfserving.api.set_credentials:Created Secret: kfserving-secret-6tv6l in namespace kubeflow
INFO:kfserving.api.set_credentials:Created Service account: kfserving-sa-tj444 in namespace kubeflow
```

### Parameters
Name | Type |  Description | Notes
------------ | ------------- | ------------- | -------------
platform  | str | Valid values: GCP or AWS| Required |
namespace | str | The kubenertes namespace. Defaults to current or default namespace.| Optional|
GCP_CREDS_FILE | str | The path for the gcp credentials file | Required for GCP |
AWS_ACCESS_KEY_ID  | str | AWS access key ID| Required for AWS |
AWS_SECRET_ACCESS_KEY  | str | AWS secret access key| Required for AWS|
S3_ENDPOINT  | str | The endpoint could be overridden explicitly with S3_ENDPOINT specified. | Optional for AWS |
AWS_REGION  | str | By default, regional endpoint is used for S3, with region controlled by AWS_REGION. If AWS_REGION is not specified, then us-east-1 is used| Optional for AWS|
S3_USE_HTTPS  | str | HTTPS is used to access S3 by default, unless `S3_USE_HTTPS=0` |Optional for AWS |
S3_VERIFY_SSL  | str | If HTTPS is used, SSL verification could be disabled with `S3_VERIFY_SSL=0` |Optional for AWS |


## create
> create(kfservice, namespace=None)

Create the provided KFService in the specified namespace

### Example

```python
from kubernetes import client

from kfserving import KFServingClient
from kfserving import constants
from kfserving import V1alpha1ModelSpec
from kfserving import V1alpha1TensorflowSpec
from kfserving import V1alpha1KFServiceSpec
from kfserving import V1alpha1KFService


default_model_spec = V1alpha1ModelSpec(tensorflow=V1alpha1TensorflowSpec(
    model_uri='gs://kfserving-samples/models/tensorflow/flowers'))

kfsvc = V1alpha1KFService(api_version=constants.KFSERVING_GROUP + '/' + constants.KFSERVING_VERSION,
                          kind=constants.KFSERVING_KIND,
                          metadata=client.V1ObjectMeta(name='flower-sample', namespace='kubeflow'),
                          spec=V1alpha1KFServiceSpec(default=default_model_spec))


KFServing = KFServingClient()
KFServing.create(kfsvc)
```

### Parameters
Name | Type |  Description | Notes
------------ | ------------- | ------------- | -------------
kfservice  | [V1alpha1KFService](V1alpha1KFService.md) | kfservice defination| |
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

from kfserving import V1alpha1ModelSpec
from kfserving import V1alpha1TensorflowSpec
from kfserving import V1alpha1KFServiceSpec
from kfserving import V1alpha1KFService
from kfserving import KFServingClient

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

KFServing = KFServingClient()
KFServing.patch('flower-sample', kfsvc)
```

### Parameters
Name | Type |  Description | Notes
------------ | ------------- | ------------- | -------------
kfservice  | [V1alpha1KFService](V1alpha1KFService.md) | kfservice defination| |
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
