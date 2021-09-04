# V1alpha2AlibiExplainerSpec

AlibiExplainerSpec defines the arguments for configuring an Alibi Explanation Server
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**config** | **dict(str, str)** | Inline custom parameter settings for explainer | [optional] 
**resources** | [**V1ResourceRequirements**](https://github.com/kubernetes-client/python/blob/master/kubernetes/docs/V1ResourceRequirements.md) | Defaults to requests and limits of 1CPU, 2Gb MEM. | [optional] 
**runtime_version** | **str** | Alibi docker image version which defaults to latest release | [optional] 
**storage_uri** | **str** | The location of a trained explanation model | [optional] 
**type** | **str** | The type of Alibi explainer | 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


