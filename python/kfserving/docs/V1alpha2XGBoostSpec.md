# V1alpha2XGBoostSpec

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**nthread** | **int** | Number of thread to be used by XGBoost | [optional]
**resources** | [**V1ResourceRequirements**](https://github.com/kubernetes-client/python/blob/master/kubernetes/docs/V1ResourceRequirements.md) | Defaults to requests and limits of 1CPU, 2Gb MEM. | [optional] 
**runtime_version** | **str** | Allowed runtime versions are specified in the inferenceservice config map | [optional] 
**storage_uri** | **str** | The location of the trained model | 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


