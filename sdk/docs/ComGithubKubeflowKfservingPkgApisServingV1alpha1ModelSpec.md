# ComGithubKubeflowKfservingPkgApisServingV1alpha1ModelSpec

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**custom** | [**ComGithubKubeflowKfservingPkgApisServingV1alpha1CustomSpec**](ComGithubKubeflowKfservingPkgApisServingV1alpha1CustomSpec.md) | The following fields follow a \&quot;1-of\&quot; semantic. Users must specify exactly one spec. | [optional] 
**max_replicas** | **int** | This is the up bound for autoscaler to scale to | [optional] 
**min_replicas** | **int** | Minimum number of replicas, pods won&#39;t scale down to 0 in case of no traffic | [optional] 
**pytorch** | [**ComGithubKubeflowKfservingPkgApisServingV1alpha1PyTorchSpec**](ComGithubKubeflowKfservingPkgApisServingV1alpha1PyTorchSpec.md) |  | [optional] 
**service_account_name** | **str** | Service Account Name | [optional] 
**sklearn** | [**ComGithubKubeflowKfservingPkgApisServingV1alpha1SKLearnSpec**](ComGithubKubeflowKfservingPkgApisServingV1alpha1SKLearnSpec.md) |  | [optional] 
**tensorflow** | [**ComGithubKubeflowKfservingPkgApisServingV1alpha1TensorflowSpec**](ComGithubKubeflowKfservingPkgApisServingV1alpha1TensorflowSpec.md) |  | [optional] 
**tensorrt** | [**ComGithubKubeflowKfservingPkgApisServingV1alpha1TensorRTSpec**](ComGithubKubeflowKfservingPkgApisServingV1alpha1TensorRTSpec.md) |  | [optional] 
**xgboost** | [**ComGithubKubeflowKfservingPkgApisServingV1alpha1XGBoostSpec**](ComGithubKubeflowKfservingPkgApisServingV1alpha1XGBoostSpec.md) |  | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


