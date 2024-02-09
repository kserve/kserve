# V1alpha1ModelSpec

ModelSpec describes a TrainedModel

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**framework** | **str** | Machine Learning &lt;framework name&gt; The values could be: \&quot;tensorflow\&quot;,\&quot;pytorch\&quot;,\&quot;sklearn\&quot;,\&quot;onnx\&quot;,\&quot;xgboost\&quot;, \&quot;myawesomeinternalframework\&quot; etc. | [default to '']
**memory** | [**ResourceQuantity**](ResourceQuantity.md) |  | 
**storage_uri** | **str** | Storage URI for the model repository | [default to '']

## Example

```python
from kserve.models.v1alpha1_model_spec import V1alpha1ModelSpec

# TODO update the JSON string below
json = "{}"
# create an instance of V1alpha1ModelSpec from a JSON string
v1alpha1_model_spec_instance = V1alpha1ModelSpec.from_json(json)
# print the JSON string representation of the object
print V1alpha1ModelSpec.to_json()

# convert the object into a dict
v1alpha1_model_spec_dict = v1alpha1_model_spec_instance.to_dict()
# create an instance of V1alpha1ModelSpec from a dict
v1alpha1_model_spec_form_dict = v1alpha1_model_spec.from_dict(v1alpha1_model_spec_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


