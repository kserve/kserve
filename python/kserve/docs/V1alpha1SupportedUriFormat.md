# V1alpha1SupportedUriFormat

SupportedUriFormat can be either prefix or regex. Todo: Add validation that only one of them is set.

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**prefix** | **str** |  | [optional] 
**regex** | **str** |  | [optional] 

## Example

```python
from kserve.models.v1alpha1_supported_uri_format import V1alpha1SupportedUriFormat

# TODO update the JSON string below
json = "{}"
# create an instance of V1alpha1SupportedUriFormat from a JSON string
v1alpha1_supported_uri_format_instance = V1alpha1SupportedUriFormat.from_json(json)
# print the JSON string representation of the object
print V1alpha1SupportedUriFormat.to_json()

# convert the object into a dict
v1alpha1_supported_uri_format_dict = v1alpha1_supported_uri_format_instance.to_dict()
# create an instance of V1alpha1SupportedUriFormat from a dict
v1alpha1_supported_uri_format_form_dict = v1alpha1_supported_uri_format.from_dict(v1alpha1_supported_uri_format_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


