# V1alpha2ModelSpec

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**custom** | [**V1alpha2CustomSpec**](V1alpha2CustomSpec.md) | The following fields follow a \&quot;1-of\&quot; semantic. Users must specify exactly one openapispec. | [optional] 
**explain** | [**V1alpha2ExplainSpec**](V1alpha2ExplainSpec.md) | Optional Explain specification to add a model explainer next to the chosen predictor. In future v1alpha2 the above model predictors would be moved down a level. | [optional] 
**max_replicas** | **int** | This is the up bound for autoscaler to scale to | [optional] 
**min_replicas** | **int** | Minimum number of replicas, pods won&#39;t scale down to 0 in case of no traffic | [optional] 
**pytorch** | [**V1alpha2PyTorchSpec**](V1alpha2PyTorchSpec.md) |  | [optional] 
**service_account_name** | **str** |  | [optional] 
**sklearn** | [**V1alpha2SKLearnSpec**](V1alpha2SKLearnSpec.md) |  | [optional] 
**tensorflow** | [**V1alpha2TensorflowSpec**](V1alpha2TensorflowSpec.md) |  | [optional] 
**tensorrt** | [**V1alpha2TensorRTSpec**](V1alpha2TensorRTSpec.md) |  | [optional] 
**xgboost** | [**V1alpha2XGBoostSpec**](V1alpha2XGBoostSpec.md) |  | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


