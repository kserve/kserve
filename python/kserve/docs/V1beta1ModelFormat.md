# V1beta1ModelFormat


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**name** | **str** | Name of the model format. | [optional] [default to '']
**version** | **str** | Version of the model format. Used in validating that a predictor is supported by a runtime. Can be \&quot;major\&quot;, \&quot;major.minor\&quot; or \&quot;major.minor.patch\&quot;. | [optional] 

## Example

```python
from kserve.models.v1beta1_model_format import V1beta1ModelFormat

# TODO update the JSON string below
json = "{}"
# create an instance of V1beta1ModelFormat from a JSON string
v1beta1_model_format_instance = V1beta1ModelFormat.from_json(json)
# print the JSON string representation of the object
print V1beta1ModelFormat.to_json()

# convert the object into a dict
v1beta1_model_format_dict = v1beta1_model_format_instance.to_dict()
# create an instance of V1beta1ModelFormat from a dict
v1beta1_model_format_form_dict = v1beta1_model_format.from_dict(v1beta1_model_format_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


