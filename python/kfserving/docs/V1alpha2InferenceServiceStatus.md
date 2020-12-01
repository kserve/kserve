# V1alpha2InferenceServiceStatus

InferenceServiceStatus defines the observed state of InferenceService
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**address** | [**KnativeAddressable**](KnativeAddressable.md) | Addressable URL for eventing | [optional] 
**canary** | [**dict(str, V1alpha2StatusConfigurationSpec)**](V1alpha2StatusConfigurationSpec.md) | Statuses for the canary endpoints of the InferenceService | [optional] 
**canary_traffic** | **int** | Traffic percentage that goes to canary services | [optional] 
**conditions** | [**list[KnativeCondition]**](KnativeCondition.md) | Conditions the latest available observations of a resource&#39;s current state. | [optional] 
**default** | [**dict(str, V1alpha2StatusConfigurationSpec)**](V1alpha2StatusConfigurationSpec.md) | Statuses for the default endpoints of the InferenceService | [optional] 
**observed_generation** | **int** | ObservedGeneration is the &#39;Generation&#39; of the Service that was last processed by the controller. | [optional] 
**traffic** | **int** | Traffic percentage that goes to default services | [optional] 
**url** | **str** | URL of the InferenceService | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


