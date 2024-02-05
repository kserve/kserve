# V1alpha1ServingRuntime

ServingRuntime is the Schema for the servingruntimes API

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**api_version** | **str** | APIVersion defines the versioned schema of this representation of an object. Servers should convert recognized schemas to the latest internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources | [optional] 
**kind** | **str** | Kind is a string value representing the REST resource this object represents. Servers may infer this from the endpoint the client submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds | [optional] 
**metadata** | [**V1ObjectMeta**](https://github.com/kubernetes-client/python/blob/master/kubernetes/docs/V1ObjectMeta.md) |  | [optional] 
**spec** | [**V1alpha1ServingRuntimeSpec**](V1alpha1ServingRuntimeSpec.md) |  | [optional] 
**status** | **object** | ServingRuntimeStatus defines the observed state of ServingRuntime | [optional] 

## Example

```python
from kserve.models.v1alpha1_serving_runtime import V1alpha1ServingRuntime

# TODO update the JSON string below
json = "{}"
# create an instance of V1alpha1ServingRuntime from a JSON string
v1alpha1_serving_runtime_instance = V1alpha1ServingRuntime.from_json(json)
# print the JSON string representation of the object
print V1alpha1ServingRuntime.to_json()

# convert the object into a dict
v1alpha1_serving_runtime_dict = v1alpha1_serving_runtime_instance.to_dict()
# create an instance of V1alpha1ServingRuntime from a dict
v1alpha1_serving_runtime_form_dict = v1alpha1_serving_runtime.from_dict(v1alpha1_serving_runtime_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


