# V1alpha1InferenceGraphStatus

InferenceGraphStatus defines the InferenceGraph conditions and status

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**annotations** | **Dict[str, str]** | Annotations is additional Status fields for the Resource to save some additional State as well as convey more information to the user. This is roughly akin to Annotations on any k8s resource, just the reconciler conveying richer information outwards. | [optional] 
**conditions** | [**List[KnativeCondition]**](KnativeCondition.md) | Conditions the latest available observations of a resource&#39;s current state. | [optional] 
**observed_generation** | **int** | ObservedGeneration is the &#39;Generation&#39; of the Service that was last processed by the controller. | [optional] 
**url** | [**KnativeURL**](KnativeURL.md) |  | [optional] 

## Example

```python
from kserve.models.v1alpha1_inference_graph_status import V1alpha1InferenceGraphStatus

# TODO update the JSON string below
json = "{}"
# create an instance of V1alpha1InferenceGraphStatus from a JSON string
v1alpha1_inference_graph_status_instance = V1alpha1InferenceGraphStatus.from_json(json)
# print the JSON string representation of the object
print V1alpha1InferenceGraphStatus.to_json()

# convert the object into a dict
v1alpha1_inference_graph_status_dict = v1alpha1_inference_graph_status_instance.to_dict()
# create an instance of V1alpha1InferenceGraphStatus from a dict
v1alpha1_inference_graph_status_form_dict = v1alpha1_inference_graph_status.from_dict(v1alpha1_inference_graph_status_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


