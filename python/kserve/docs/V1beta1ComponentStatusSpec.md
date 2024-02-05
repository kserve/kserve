# V1beta1ComponentStatusSpec

ComponentStatusSpec describes the state of the component

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**address** | [**KnativeAddressable**](KnativeAddressable.md) |  | [optional] 
**grpc_url** | [**KnativeURL**](KnativeURL.md) |  | [optional] 
**latest_created_revision** | **str** | Latest revision name that is created | [optional] 
**latest_ready_revision** | **str** | Latest revision name that is in ready state | [optional] 
**latest_rolledout_revision** | **str** | Latest revision name that is rolled out with 100 percent traffic | [optional] 
**previous_rolledout_revision** | **str** | Previous revision name that is rolled out with 100 percent traffic | [optional] 
**rest_url** | [**KnativeURL**](KnativeURL.md) |  | [optional] 
**traffic** | [**List[KnativeDevServingPkgApisServingV1TrafficTarget]**](KnativeDevServingPkgApisServingV1TrafficTarget.md) | Traffic holds the configured traffic distribution for latest ready revision and previous rolled out revision. | [optional] 
**url** | [**KnativeURL**](KnativeURL.md) |  | [optional] 

## Example

```python
from kserve.models.v1beta1_component_status_spec import V1beta1ComponentStatusSpec

# TODO update the JSON string below
json = "{}"
# create an instance of V1beta1ComponentStatusSpec from a JSON string
v1beta1_component_status_spec_instance = V1beta1ComponentStatusSpec.from_json(json)
# print the JSON string representation of the object
print V1beta1ComponentStatusSpec.to_json()

# convert the object into a dict
v1beta1_component_status_spec_dict = v1beta1_component_status_spec_instance.to_dict()
# create an instance of V1beta1ComponentStatusSpec from a dict
v1beta1_component_status_spec_form_dict = v1beta1_component_status_spec.from_dict(v1beta1_component_status_spec_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


