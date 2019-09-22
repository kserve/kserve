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
KFServingClient | [set_credentials](#set_credentials) | Set Credentials|
KFServingClient | [create](#create) | Create KFService|
KFServingClient | [get](#get)    | Get or watch the specified KFService or all KFServices in the namespace |
KFServingClient | [patch](#patch)  | Patch the specified KFService|
KFServingClient | [delete](#delete) | Delete the specified KFService |

## set_credentials
> set_credentials(storage_type, namespace=None, credentials_file=None, service_account='kfserving-service-credentials', **kwargs):

Create or update a `Secret` and `Service Account` for GCS and S3 for the provided credentials. Once the `Service Account` is applied, it may be used in the `Service Account` field of a KFService's [V1alpha2ModelSpec](V1alpha2ModelSpec.md).

### Example

Example for creating GCP credentials.
```python
from kfserving import KFServingClient

KFServing = KFServingClient()
KFServing.set_credentials(storage_type='GCS',
                          namespace='kubeflow',
                          credentials_file='/tmp/gcp.json',
                          service_account='user_specified_sa_name')
```

The API supports specifying a Service Account by `service_account`, or using default Service Account `kfserving-service-credentials`, if the Service Account does not exist, the API will create it and attach the created secret with the Service Account, if exists, only patch it to attach the created Secret.

Example for creating S3 credentials.
```python
from kfserving import KFServingClient

KFServing = KFServingClient()
KFServing.set_credentials(storage_type='S3',
                          namespace='kubeflow',
                          credentials_file='/tmp/awcredentials',
                          s3_profile='default',
                          s3_endpoint='s3.us-west-amazonaws.com',
                          s3_region='us-west-2',
                          s3_use_https='1',
                          s3_verify_ssl='0')
```

Example for creating Azure credentials.
```python
from kfserving import KFServingClient

KFServing = KFServingClient()
KFServing.set_credentials(storage_type='Azure',
                          namespace='kubeflow',
                          credentials_file='/path/azure_credentials.json')
```

The created or patched `Secret` and `Service Account` will be shown as following:
```
INFO:kfserving.api.set_credentials:Created Secret: kfserving-secret-6tv6l in namespace kubeflow
INFO:kfserving.api.set_credentials:Created (or Patched) Service account: kfserving-service-credentials in namespace kubeflow
```

### Parameters
Name | Type | Storage Type | Description
------------ | ------------- | ------------- | -------------
storage_type | str | All |Required. Valid values: GCS, S3 or Azure |
namespace | str | All |Optional. The kubernetes namespace. Defaults to current or default namespace.|
credentials_file | str | All |Optional. The path for the credentials file. The default file for GCS is `~/.config/gcloud/application_default_credentials.json`, see the [instructions](https://cloud.google.com/sdk/gcloud/reference/auth/application-default/login) on creating the GCS credentials file. For S3 is `~/.aws/credentials`, see the [instructions](https://docs.aws.amazon.com/sdk-for-java/v1/developer-guide/setup-credentials.html) on creating the S3 credentials file. For Azure is `~/.azure/azure_credentials.json`, see the [instructions](https://docs.microsoft.com/en-us/azure/python/python-sdk-azure-authenticate#mgmt-auth-file) on creating the Azure credentials file. |
service_account  | str | All |Optional. The name of service account. Supports specifying the `service_account`, or using default Service Account `kfserving-service-credentials`. If the Service Account does not exist, the API will create it and attach the created Secret with the Service Account, if exists, only patch it to attach the created Secret.|
s3_endpoint  | str | S3 only |Optional. The S3 endpoint. |
s3_region  | str | S3 only|Optional. The S3 region By default, regional endpoint is used for S3.| |
s3_use_https  | str | S3 only |Optional. HTTPS is used to access S3 by default, unless `s3_use_https=0` |
s3_verify_ssl  | str | S3 only|Optional. If HTTPS is used, SSL verification could be disabled with `s3_verify_ssl=0` |


## create
> create(kfservice, namespace=None, watch=False, timeout_seconds=600)

Create the provided KFService in the specified namespace

### Example

```python
from kubernetes import client

from kfserving import KFServingClient
from kfserving import constants
from kfserving import V1alpha2EndpointSpec
from kfserving import V1alpha2PredictorSpec
from kfserving import V1alpha2TensorflowSpec
from kfserving import V1alpha2KFServiceSpec
from kfserving import V1alpha2KFService


default_model_spec = V1alpha2EndpointSpec(predictor=V1alpha2PredictorSpec(tensorflow=V1alpha2TensorflowSpec(
    storage_uri='gs://kfserving-samples/models/tensorflow/flowers')))

kfsvc = V1alpha2KFService(api_version=constants.KFSERVING_GROUP + '/' + constants.KFSERVING_VERSION,
                          kind=constants.KFSERVING_KIND,
                          metadata=client.V1ObjectMeta(name='flower-sample', namespace='kubeflow'),
                          spec=V1alpha2KFServiceSpec(default=default_model_spec))


KFServing = KFServingClient()
KFServing.create(kfsvc)

# The API also supports watching the created KFService status till it's READY.
# KFServing.create(kfsvc, watch=True)
```


### Parameters
Name | Type |  Description | Notes
------------ | ------------- | ------------- | -------------
kfservice  | [V1alpha2KFService](V1alpha2KFService.md) | KFService defination| Required |
namespace | str | Namespace for KFService deploying to. If the `namespace` is not defined, will align with KFService definition, or use current or default namespace if namespace is not specified in KFService definition.  | Optional |
watch | bool | Watch the created KFService if `True`, otherwise will return the created KFService object. Stop watching if KFService reaches the optional specified `timeout_seconds` or once the KFService overall status `READY` is `True`. | Optional |
timeout_seconds | int | Timeout seconds for watching. Defaults to 600. | Optional |

### Return type
object

## get
> get(name=None, namespace=None, watch=False, timeout_seconds=600)

Get the created KFService in the specified namespace

### Example

```python
from kfserving import KFServingClient

KFServing = KFServingClient()
KFServing.get('flower-sample', namespace='kubeflow')
```
The API also support watching the specified KFService or all KFService in the namespace.
```python
from kfserving import KFServingClient

KFServing = KFServingClient()
KFServing.get('flower-sample', namespace='kubeflow', watch=True, timeout_seconds=120)
```
The outputs will be as following. Stop watching if KFService reaches the optional specified `timeout_seconds` or once the KFService overall status `READY` is `True`.
```sh
NAME                 READY      DEFAULT_TRAFFIC CANARY_TRAFFIC  URL                                               
flower-sample        Unknown                                    http://flower-sample.kubeflow.example.com         
flower-sample        Unknown    90               10             http://flower-sample.kubeflow.example.com         
flower-sample        True       90               10             http://flower-sample.kubeflow.example.com         
```


### Parameters
Name | Type |  Description | Notes
------------ | ------------- | ------------- | -------------
name  | str | KFService name. If the `name` is not specified, it will get or watch all KFServices in the namespace.| Optional. |
namespace | str | The KFService's namespace. Defaults to current or default namespace.| Optional |
watch | bool | Watch the specified KFService or all KFService in the namespace if `True`, otherwise will return object for the specified KFService or all KFService in the namespace. Stop watching if KFService reaches the optional specified `timeout_seconds` or once the speficed KFService overall status `READY` is `True` (Only if the `name` is speficed). | Optional |
timeout_seconds | int | Timeout seconds for watching. Defaults to 600. | Optional |

### Return type
object


## patch
> patch(name, kfservice, namespace=None, watch=False, timeout_seconds=600)

Patch the created KFService in the specified namespace

### Example

```python
from kubernetes import client
from kfserving import constants
from kfserving import V1alpha2EndpointSpec
from kfserving import V1alpha2PredictorSpec
from kfserving import V1alpha2TensorflowSpec
from kfserving import V1alpha2KFServiceSpec
from kfserving import V1alpha2KFService
from kfserving import KFServingClient

default_model_spec = V1alpha2EndpointSpec(predictor=V1alpha2PredictorSpec(tensorflow=V1alpha2TensorflowSpec(
    storage_uri='gs://kfserving-samples/models/tensorflow/flowers')))
canary_model_spec = V1alpha2EndpointSpec(predictor=V1alpha2PredictorSpec(tensorflow=V1alpha2TensorflowSpec(
    storage_uri='gs://kfserving-samples/models/tensorflow/flowers')))

kfsvc = V1alpha2KFService(api_version=constants.KFSERVING_GROUP + '/' + constants.KFSERVING_VERSION,
                          kind=constants.KFSERVING_KIND,
                          metadata=client.V1ObjectMeta(name='flower-sample', namespace='kubeflow'),
                          spec=V1alpha2KFServiceSpec(default=default_model_spec,
                                                     canary=canary_model_spec,
                                                     canary_traffic_percent=10))

KFServing = KFServingClient()
KFServing.patch('flower-sample', kfsvc)

# The API also supports watching the patached KFService status till it's READY.
# KFServing.patch('flower-sample', kfsvc, watch=True)
```

### Parameters
Name | Type |  Description | Notes
------------ | ------------- | ------------- | -------------
kfservice  | [V1alpha2KFService](V1alpha2KFService.md) | KFService defination| Required |
namespace | str | The KFService's namespace for patching. If the `namespace` is not defined, will align with KFService definition, or use current or default namespace if namespace is not specified in KFService definition. | Optional|
watch | bool | Watch the patched KFService if `True`, otherwise will return the patched KFService object. Stop watching if KFService reaches the optional specified `timeout_seconds` or once the KFService overall status `READY` is `True`. | Optional |
timeout_seconds | int | Timeout seconds for watching. Defaults to 600. | Optional |

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
Name  | str | KFService name| |
namespace | str | The kfservice's namespace. Defaults to current or default namespace. | Optional|

### Return type
object
