# V1beta1ModelStatus


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**copies** | [**V1beta1ModelCopies**](V1beta1ModelCopies.md) |  | [optional] 
**last_failure_info** | [**V1beta1FailureInfo**](V1beta1FailureInfo.md) |  | [optional] 
**states** | [**V1beta1ModelRevisionStates**](V1beta1ModelRevisionStates.md) |  | [optional] 
**transition_status** | **str** | Whether the available predictor endpoints reflect the current Spec or is in transition | [default to '']

## Example

```python
from kserve.models.v1beta1_model_status import V1beta1ModelStatus

# TODO update the JSON string below
json = "{}"
# create an instance of V1beta1ModelStatus from a JSON string
v1beta1_model_status_instance = V1beta1ModelStatus.from_json(json)
# print the JSON string representation of the object
print V1beta1ModelStatus.to_json()

# convert the object into a dict
v1beta1_model_status_dict = v1beta1_model_status_instance.to_dict()
# create an instance of V1beta1ModelStatus from a dict
v1beta1_model_status_form_dict = v1beta1_model_status.from_dict(v1beta1_model_status_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


