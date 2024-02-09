# V1beta1IngressConfig


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**disable_istio_virtual_host** | **bool** |  | [optional] 
**domain_template** | **str** |  | [optional] 
**ingress_class_name** | **str** |  | [optional] 
**ingress_domain** | **str** |  | [optional] 
**ingress_gateway** | **str** |  | [optional] 
**ingress_service** | **str** |  | [optional] 
**local_gateway** | **str** |  | [optional] 
**local_gateway_service** | **str** |  | [optional] 
**path_template** | **str** |  | [optional] 
**url_scheme** | **str** |  | [optional] 

## Example

```python
from kserve.models.v1beta1_ingress_config import V1beta1IngressConfig

# TODO update the JSON string below
json = "{}"
# create an instance of V1beta1IngressConfig from a JSON string
v1beta1_ingress_config_instance = V1beta1IngressConfig.from_json(json)
# print the JSON string representation of the object
print V1beta1IngressConfig.to_json()

# convert the object into a dict
v1beta1_ingress_config_dict = v1beta1_ingress_config_instance.to_dict()
# create an instance of V1beta1IngressConfig from a dict
v1beta1_ingress_config_form_dict = v1beta1_ingress_config.from_dict(v1beta1_ingress_config_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


