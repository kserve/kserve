# V1alpha2TritonSpec

TritonSpec defines arguments for configuring Triton Inference Server.
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**resources** | [**V1ResourceRequirements**](https://github.com/kubernetes-client/python/blob/master/kubernetes/docs/V1ResourceRequirements.md) | Defaults to requests and limits of 1CPU, 2Gb MEM. | [optional] 
**runtime_version** | **str** | Triton Inference Server docker image version, default version can be set in the inferenceservice configmap | [optional] 
**storage_uri** | **str** | The URI for the trained model repository(https://docs.nvidia.com/deeplearning/triton-inference-server/master-user-guide/docs/model_repository.html) | 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


