# V1beta1ExplainersConfig


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**alibi** | [**V1beta1ExplainerConfig**](V1beta1ExplainerConfig.md) |  | [optional] 
**art** | [**V1beta1ExplainerConfig**](V1beta1ExplainerConfig.md) |  | [optional] 

## Example

```python
from kserve.models.v1beta1_explainers_config import V1beta1ExplainersConfig

# TODO update the JSON string below
json = "{}"
# create an instance of V1beta1ExplainersConfig from a JSON string
v1beta1_explainers_config_instance = V1beta1ExplainersConfig.from_json(json)
# print the JSON string representation of the object
print V1beta1ExplainersConfig.to_json()

# convert the object into a dict
v1beta1_explainers_config_dict = v1beta1_explainers_config_instance.to_dict()
# create an instance of V1beta1ExplainersConfig from a dict
v1beta1_explainers_config_form_dict = v1beta1_explainers_config.from_dict(v1beta1_explainers_config_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


