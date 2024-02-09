# V1beta1DeployConfig


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**default_deployment_mode** | **str** |  | [optional] 

## Example

```python
from kserve.models.v1beta1_deploy_config import V1beta1DeployConfig

# TODO update the JSON string below
json = "{}"
# create an instance of V1beta1DeployConfig from a JSON string
v1beta1_deploy_config_instance = V1beta1DeployConfig.from_json(json)
# print the JSON string representation of the object
print V1beta1DeployConfig.to_json()

# convert the object into a dict
v1beta1_deploy_config_dict = v1beta1_deploy_config_instance.to_dict()
# create an instance of V1beta1DeployConfig from a dict
v1beta1_deploy_config_form_dict = v1beta1_deploy_config.from_dict(v1beta1_deploy_config_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


