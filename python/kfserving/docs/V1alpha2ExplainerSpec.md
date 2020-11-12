# V1alpha2ExplainerSpec

ExplainerSpec defines the arguments for a model explanation server, The following fields follow a \"1-of\" semantic. Users must specify exactly one spec.
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**aix** | [**V1alpha2AIXExplainerSpec**](V1alpha2AIXExplainerSpec.md) | Spec for AIX explainer | [optional] 
**alibi** | [**V1alpha2AlibiExplainerSpec**](V1alpha2AlibiExplainerSpec.md) | Spec for alibi explainer | [optional] 
**batcher** | [**V1alpha2Batcher**](V1alpha2Batcher.md) | Activate request batching | [optional] 
**custom** | [**V1alpha2CustomSpec**](V1alpha2CustomSpec.md) | Spec for a custom explainer | [optional] 
**logger** | [**V1alpha2Logger**](V1alpha2Logger.md) | Activate request/response logging | [optional] 
**max_replicas** | **int** | This is the up bound for autoscaler to scale to | [optional] 
**min_replicas** | **int** | Minimum number of replicas which defaults to 1, when minReplicas &#x3D; 0 pods scale down to 0 in case of no traffic | [optional] 
**parallelism** | **int** | Parallelism specifies how many requests can be processed concurrently, this sets the hard limit of the container concurrency(https://knative.dev/docs/serving/autoscaling/concurrency). | [optional] 
**service_account_name** | **str** | ServiceAccountName is the name of the ServiceAccount to use to run the service | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


