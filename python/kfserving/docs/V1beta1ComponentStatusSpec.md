# V1beta1ComponentStatusSpec

ComponentStatusSpec describes the state of the component
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**address** | [**KnativeAddressable**](KnativeAddressable.md) |  | [optional] 
**latest_created_revision** | **str** | Latest revision name that is in created | [optional] 
**latest_ready_revision** | **str** | Latest revision name that is in ready state | [optional] 
**previous_ready_revision** | **str** | Previous revision name that is in ready state | [optional] 
**traffic_percent** | **int** | Traffic percent on the latest ready revision | [optional] 
**url** | [**KnativeURL**](KnativeURL.md) |  | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


