# V1beta1Batcher

Batcher specifies optional payload batching available for all components

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**max_batch_size** | **int** | Specifies the max number of requests to trigger a batch | [optional] 
**max_latency** | **int** | Specifies the max latency to trigger a batch | [optional] 
**timeout** | **int** | Specifies the timeout of a batch | [optional] 

## Example

```python
from kserve.models.v1beta1_batcher import V1beta1Batcher

# TODO update the JSON string below
json = "{}"
# create an instance of V1beta1Batcher from a JSON string
v1beta1_batcher_instance = V1beta1Batcher.from_json(json)
# print the JSON string representation of the object
print V1beta1Batcher.to_json()

# convert the object into a dict
v1beta1_batcher_dict = v1beta1_batcher_instance.to_dict()
# create an instance of V1beta1Batcher from a dict
v1beta1_batcher_form_dict = v1beta1_batcher.from_dict(v1beta1_batcher_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


