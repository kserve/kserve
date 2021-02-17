# V1alpha2XGBoostSpec

XGBoostSpec defines arguments for configuring XGBoost model serving.
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**nthread** | **int** | Number of thread to be used by XGBoost | [optional] 
**resources** | [**V1ResourceRequirements**](https://github.com/kubernetes-client/python/blob/master/kubernetes/docs/V1ResourceRequirements.md) | Defaults to requests and limits of 1CPU, 2Gb MEM. | [optional] 
**runtime_version** | **str** | XGBoost KFServer docker image version which defaults to latest release | [optional] 
**storage_uri** | **str** | The URI of the trained model which contains model.bst | 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


