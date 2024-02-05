# V1alpha1InferenceTarget

Exactly one InferenceTarget field must be specified

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**node_name** | **str** | The node name for routing as next step | [optional] 
**service_name** | **str** | named reference for InferenceService | [optional] 
**service_url** | **str** | InferenceService URL, mutually exclusive with ServiceName | [optional] 

## Example

```python
from kserve.models.v1alpha1_inference_target import V1alpha1InferenceTarget

# TODO update the JSON string below
json = "{}"
# create an instance of V1alpha1InferenceTarget from a JSON string
v1alpha1_inference_target_instance = V1alpha1InferenceTarget.from_json(json)
# print the JSON string representation of the object
print V1alpha1InferenceTarget.to_json()

# convert the object into a dict
v1alpha1_inference_target_dict = v1alpha1_inference_target_instance.to_dict()
# create an instance of V1alpha1InferenceTarget from a dict
v1alpha1_inference_target_form_dict = v1alpha1_inference_target.from_dict(v1alpha1_inference_target_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


