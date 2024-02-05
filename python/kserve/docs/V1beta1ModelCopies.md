# V1beta1ModelCopies


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**failed_copies** | **int** | How many copies of this predictor&#39;s models failed to load recently | [default to 0]
**total_copies** | **int** | Total number copies of this predictor&#39;s models that are currently loaded | [optional] 

## Example

```python
from kserve.models.v1beta1_model_copies import V1beta1ModelCopies

# TODO update the JSON string below
json = "{}"
# create an instance of V1beta1ModelCopies from a JSON string
v1beta1_model_copies_instance = V1beta1ModelCopies.from_json(json)
# print the JSON string representation of the object
print V1beta1ModelCopies.to_json()

# convert the object into a dict
v1beta1_model_copies_dict = v1beta1_model_copies_instance.to_dict()
# create an instance of V1beta1ModelCopies from a dict
v1beta1_model_copies_form_dict = v1beta1_model_copies.from_dict(v1beta1_model_copies_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


