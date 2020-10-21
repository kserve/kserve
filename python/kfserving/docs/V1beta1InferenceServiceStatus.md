# V1beta1InferenceServiceStatus

InferenceServiceStatus defines the observed state of InferenceService
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**address** | [**KnativeAddressable**](KnativeAddressable.md) |  | [optional] 
**components** | [**dict(str, V1beta1ComponentStatusSpec)**](V1beta1ComponentStatusSpec.md) | Statuses for the components of the InferenceService | [optional] 
**conditions** | [**list[KnativeCondition]**](KnativeCondition.md) | Conditions the latest available observations of a resource&#39;s current state. | [optional] 
**observed_generation** | **int** | ObservedGeneration is the &#39;Generation&#39; of the Service that was last processed by the controller. | [optional] 
**url** | [**KnativeURL**](KnativeURL.md) |  | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


