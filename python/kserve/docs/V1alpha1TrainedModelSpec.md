# V1alpha1TrainedModelSpec

TrainedModelSpec defines the TrainedModel spec

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**inference_service** | **str** | parent inference service to deploy to | [default to '']
**model** | [**V1alpha1ModelSpec**](V1alpha1ModelSpec.md) |  | 

## Example

```python
from kserve.models.v1alpha1_trained_model_spec import V1alpha1TrainedModelSpec

# TODO update the JSON string below
json = "{}"
# create an instance of V1alpha1TrainedModelSpec from a JSON string
v1alpha1_trained_model_spec_instance = V1alpha1TrainedModelSpec.from_json(json)
# print the JSON string representation of the object
print V1alpha1TrainedModelSpec.to_json()

# convert the object into a dict
v1alpha1_trained_model_spec_dict = v1alpha1_trained_model_spec_instance.to_dict()
# create an instance of V1alpha1TrainedModelSpec from a dict
v1alpha1_trained_model_spec_form_dict = v1alpha1_trained_model_spec.from_dict(v1alpha1_trained_model_spec_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


