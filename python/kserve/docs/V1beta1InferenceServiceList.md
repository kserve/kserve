# V1beta1InferenceServiceList

InferenceServiceList contains a list of Service

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**api_version** | **str** | APIVersion defines the versioned schema of this representation of an object. Servers should convert recognized schemas to the latest internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources | [optional] 
**items** | [**List[V1beta1InferenceService]**](V1beta1InferenceService.md) |  | 
**kind** | **str** | Kind is a string value representing the REST resource this object represents. Servers may infer this from the endpoint the client submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds | [optional] 
**metadata** | [**V1ListMeta**](https://github.com/kubernetes-client/python/blob/master/kubernetes/docs/V1ListMeta.md) |  | [optional] 

## Example

```python
from kserve.models.v1beta1_inference_service_list import V1beta1InferenceServiceList

# TODO update the JSON string below
json = "{}"
# create an instance of V1beta1InferenceServiceList from a JSON string
v1beta1_inference_service_list_instance = V1beta1InferenceServiceList.from_json(json)
# print the JSON string representation of the object
print V1beta1InferenceServiceList.to_json()

# convert the object into a dict
v1beta1_inference_service_list_dict = v1beta1_inference_service_list_instance.to_dict()
# create an instance of V1beta1InferenceServiceList from a dict
v1beta1_inference_service_list_form_dict = v1beta1_inference_service_list.from_dict(v1beta1_inference_service_list_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


