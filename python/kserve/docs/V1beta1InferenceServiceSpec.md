# V1beta1InferenceServiceSpec

InferenceServiceSpec is the top level type for this resource

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**explainer** | [**V1beta1ExplainerSpec**](V1beta1ExplainerSpec.md) |  | [optional] 
**predictor** | [**V1beta1PredictorSpec**](V1beta1PredictorSpec.md) |  | 
**transformer** | [**V1beta1TransformerSpec**](V1beta1TransformerSpec.md) |  | [optional] 

## Example

```python
from kserve.models.v1beta1_inference_service_spec import V1beta1InferenceServiceSpec

# TODO update the JSON string below
json = "{}"
# create an instance of V1beta1InferenceServiceSpec from a JSON string
v1beta1_inference_service_spec_instance = V1beta1InferenceServiceSpec.from_json(json)
# print the JSON string representation of the object
print V1beta1InferenceServiceSpec.to_json()

# convert the object into a dict
v1beta1_inference_service_spec_dict = v1beta1_inference_service_spec_instance.to_dict()
# create an instance of V1beta1InferenceServiceSpec from a dict
v1beta1_inference_service_spec_form_dict = v1beta1_inference_service_spec.from_dict(v1beta1_inference_service_spec_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


