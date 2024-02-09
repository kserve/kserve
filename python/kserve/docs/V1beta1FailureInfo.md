# V1beta1FailureInfo


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**exit_code** | **int** | Exit status from the last termination of the container | [optional] 
**location** | **str** | Name of component to which the failure relates (usually Pod name) | [optional] 
**message** | **str** | Detailed error message | [optional] 
**model_revision_name** | **str** | Internal Revision/ID of model, tied to specific Spec contents | [optional] 
**reason** | **str** | High level class of failure | [optional] 
**time** | [**V1Time**](V1Time.md) |  | [optional] 

## Example

```python
from kserve.models.v1beta1_failure_info import V1beta1FailureInfo

# TODO update the JSON string below
json = "{}"
# create an instance of V1beta1FailureInfo from a JSON string
v1beta1_failure_info_instance = V1beta1FailureInfo.from_json(json)
# print the JSON string representation of the object
print V1beta1FailureInfo.to_json()

# convert the object into a dict
v1beta1_failure_info_dict = v1beta1_failure_info_instance.to_dict()
# create an instance of V1beta1FailureInfo from a dict
v1beta1_failure_info_form_dict = v1beta1_failure_info.from_dict(v1beta1_failure_info_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


