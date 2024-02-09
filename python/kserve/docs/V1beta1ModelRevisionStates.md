# V1beta1ModelRevisionStates


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**active_model_state** | **str** | High level state string: Pending, Standby, Loading, Loaded, FailedToLoad | [default to '']
**target_model_state** | **str** |  | [optional] 

## Example

```python
from kserve.models.v1beta1_model_revision_states import V1beta1ModelRevisionStates

# TODO update the JSON string below
json = "{}"
# create an instance of V1beta1ModelRevisionStates from a JSON string
v1beta1_model_revision_states_instance = V1beta1ModelRevisionStates.from_json(json)
# print the JSON string representation of the object
print V1beta1ModelRevisionStates.to_json()

# convert the object into a dict
v1beta1_model_revision_states_dict = v1beta1_model_revision_states_instance.to_dict()
# create an instance of V1beta1ModelRevisionStates from a dict
v1beta1_model_revision_states_form_dict = v1beta1_model_revision_states.from_dict(v1beta1_model_revision_states_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


