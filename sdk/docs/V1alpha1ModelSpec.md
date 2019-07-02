# V1alpha1ModelSpec

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**custom** | [**V1alpha1CustomSpec**](V1alpha1CustomSpec.md) | The following fields follow a \&quot;1-of\&quot; semantic. Users must specify exactly one openapispec. | [optional] 
**max_replicas** | **int** | This is the up bound for autoscaler to scale to | [optional] 
**min_replicas** | **int** | Minimum number of replicas, pods won&#39;t scale down to 0 in case of no traffic | [optional] 
**pytorch** | [**V1alpha1PyTorchSpec**](V1alpha1PyTorchSpec.md) |  | [optional] 
**service_account_name** | **str** | Service Account Name | [optional] 
**sklearn** | [**V1alpha1SKLearnSpec**](V1alpha1SKLearnSpec.md) |  | [optional] 
**tensorflow** | [**V1alpha1TensorflowSpec**](V1alpha1TensorflowSpec.md) |  | [optional] 
**tensorrt** | [**V1alpha1TensorRTSpec**](V1alpha1TensorRTSpec.md) |  | [optional] 
**xgboost** | [**V1alpha1XGBoostSpec**](V1alpha1XGBoostSpec.md) |  | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


