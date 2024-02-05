# V1beta1ExplainerConfig


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**default_image_version** | **str** | default explainer docker image version | [default to '']
**image** | **str** | explainer docker image name | [default to '']

## Example

```python
from kserve.models.v1beta1_explainer_config import V1beta1ExplainerConfig

# TODO update the JSON string below
json = "{}"
# create an instance of V1beta1ExplainerConfig from a JSON string
v1beta1_explainer_config_instance = V1beta1ExplainerConfig.from_json(json)
# print the JSON string representation of the object
print V1beta1ExplainerConfig.to_json()

# convert the object into a dict
v1beta1_explainer_config_dict = v1beta1_explainer_config_instance.to_dict()
# create an instance of V1beta1ExplainerConfig from a dict
v1beta1_explainer_config_form_dict = v1beta1_explainer_config.from_dict(v1beta1_explainer_config_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


