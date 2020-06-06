# V1alpha2ExplainerSpec

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**alibi** | [**V1alpha2AlibiExplainerSpec**](V1alpha2AlibiExplainerSpec.md) | Spec for alibi explainer | [optional] 
**custom** | [**V1alpha2CustomSpec**](V1alpha2CustomSpec.md) | Spec for a custom explainer | [optional] 
**logger** | [**V1alpha2Logger**](V1alpha2Logger.md) | Activate request/response logging | [optional] 
**max_replicas** | **int** | This is the up bound for autoscaler to scale to | [optional] 
**min_replicas** | **int** | Minimum number of replicas, pods won&#39;t scale down to 0 in case of no traffic | [optional] 
**parallelism** | **int** | Parallelism specifies how many requests can be processed concurrently, this sets the target concurrency for Autoscaling(KPA). For model servers that support tuning parallelism will use this value, by default the parallelism is the number of the CPU cores for most of the model servers. | [optional] 
**service_account_name** | **str** | ServiceAccountName is the name of the ServiceAccount to use to run the service | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


