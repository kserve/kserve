# V1alpha1BuiltInAdapter

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**env** | [**list[V1EnvVar]**](https://github.com/kubernetes-client/python/blob/master/kubernetes/docs/V1EnvVar.md) | Environment variables used to control other aspects of the built-in adapter&#39;s behaviour (uncommon) | [optional] 
**mem_buffer_bytes** | **int** | Fixed memory overhead to subtract from runtime container&#39;s memory allocation to determine model capacity | [optional] 
**model_loading_timeout_millis** | **int** | Timeout for model loading operations in milliseconds | [optional] 
**runtime_management_port** | **int** | Port which the runtime server listens for model management requests | [optional] 
**server_type** | **str** | ServerType can be one of triton/mlserver and the runtime&#39;s container must have the same name | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


