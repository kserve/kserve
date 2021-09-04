# V1alpha2InferenceServiceSpec

InferenceServiceSpec defines the desired state of InferenceService
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**canary** | [**V1alpha2EndpointSpec**](V1alpha2EndpointSpec.md) | Canary defines alternate endpoints to route a percentage of traffic. | [optional] 
**canary_traffic_percent** | **int** | CanaryTrafficPercent defines the percentage of traffic going to canary InferenceService endpoints | [optional] 
**default** | [**V1alpha2EndpointSpec**](V1alpha2EndpointSpec.md) | Default defines default InferenceService endpoints | 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


