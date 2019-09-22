# V1alpha2ExplainerSpec

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**alibi** | [**V1alpha2AlibiExplainerSpec**](V1alpha2AlibiExplainerSpec.md) | The following fields follow a \&quot;1-of\&quot; semantic. Users must specify exactly one openapispec. | [optional] 
**custom** | [**V1alpha2CustomSpec**](V1alpha2CustomSpec.md) |  | [optional] 
**max_replicas** | **int** | This is the up bound for autoscaler to scale to | [optional] 
**min_replicas** | **int** | Minimum number of replicas, pods won&#39;t scale down to 0 in case of no traffic | [optional] 
**service_account_name** | **str** | ServiceAccountName is the name of the ServiceAccount to use to run the service | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


