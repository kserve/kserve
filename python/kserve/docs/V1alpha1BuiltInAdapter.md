# V1alpha1BuiltInAdapter


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**env** | [**List[V1EnvVar]**](https://github.com/kubernetes-client/python/blob/master/kubernetes/docs/V1EnvVar.md) | Environment variables used to control other aspects of the built-in adapter&#39;s behaviour (uncommon) | [optional] 
**mem_buffer_bytes** | **int** | Fixed memory overhead to subtract from runtime container&#39;s memory allocation to determine model capacity | [optional] 
**model_loading_timeout_millis** | **int** | Timeout for model loading operations in milliseconds | [optional] 
**runtime_management_port** | **int** | Port which the runtime server listens for model management requests | [optional] 
**server_type** | **str** | ServerType must be one of the supported built-in types such as \&quot;triton\&quot; or \&quot;mlserver\&quot;, and the runtime&#39;s container must have the same name | [optional] 

## Example

```python
from kserve.models.v1alpha1_built_in_adapter import V1alpha1BuiltInAdapter

# TODO update the JSON string below
json = "{}"
# create an instance of V1alpha1BuiltInAdapter from a JSON string
v1alpha1_built_in_adapter_instance = V1alpha1BuiltInAdapter.from_json(json)
# print the JSON string representation of the object
print V1alpha1BuiltInAdapter.to_json()

# convert the object into a dict
v1alpha1_built_in_adapter_dict = v1alpha1_built_in_adapter_instance.to_dict()
# create an instance of V1alpha1BuiltInAdapter from a dict
v1alpha1_built_in_adapter_form_dict = v1alpha1_built_in_adapter.from_dict(v1alpha1_built_in_adapter_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


