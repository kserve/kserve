# V1beta1InferenceService

InferenceService is the Schema for the InferenceServices API

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**api_version** | **str** | APIVersion defines the versioned schema of this representation of an object. Servers should convert recognized schemas to the latest internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources | [optional] 
**kind** | **str** | Kind is a string value representing the REST resource this object represents. Servers may infer this from the endpoint the client submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds | [optional] 
**metadata** | [**V1ObjectMeta**](https://github.com/kubernetes-client/python/blob/master/kubernetes/docs/V1ObjectMeta.md) |  | [optional] 
**spec** | [**V1beta1InferenceServiceSpec**](V1beta1InferenceServiceSpec.md) |  | [optional] 
**status** | [**V1beta1InferenceServiceStatus**](V1beta1InferenceServiceStatus.md) |  | [optional] 

## Example

```python
from kserve.models.v1beta1_inference_service import V1beta1InferenceService

# TODO update the JSON string below
json = "{}"
# create an instance of V1beta1InferenceService from a JSON string
v1beta1_inference_service_instance = V1beta1InferenceService.from_json(json)
# print the JSON string representation of the object
print V1beta1InferenceService.to_json()

# convert the object into a dict
v1beta1_inference_service_dict = v1beta1_inference_service_instance.to_dict()
# create an instance of V1beta1InferenceService from a dict
v1beta1_inference_service_form_dict = v1beta1_inference_service.from_dict(v1beta1_inference_service_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


