# KfservingV1alpha1ModelSpec

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**custom** | [**KfservingV1alpha1CustomSpec**](KfservingV1alpha1CustomSpec.md) | The following fields follow a \&quot;1-of\&quot; semantic. Users must specify exactly one openapispec. | [optional] 
**max_replicas** | **int** | This is the up bound for autoscaler to scale to | [optional] 
**min_replicas** | **int** | Minimum number of replicas, pods won&#39;t scale down to 0 in case of no traffic | [optional] 
**pytorch** | [**KfservingV1alpha1PyTorchSpec**](KfservingV1alpha1PyTorchSpec.md) |  | [optional] 
**service_account_name** | **str** | Service Account Name | [optional] 
**sklearn** | [**KfservingV1alpha1SKLearnSpec**](KfservingV1alpha1SKLearnSpec.md) |  | [optional] 
**tensorflow** | [**KfservingV1alpha1TensorflowSpec**](KfservingV1alpha1TensorflowSpec.md) |  | [optional] 
**tensorrt** | [**KfservingV1alpha1TensorRTSpec**](KfservingV1alpha1TensorRTSpec.md) |  | [optional] 
**xgboost** | [**KfservingV1alpha1XGBoostSpec**](KfservingV1alpha1XGBoostSpec.md) |  | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


