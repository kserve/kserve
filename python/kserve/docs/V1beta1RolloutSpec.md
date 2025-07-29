# V1beta1RolloutSpec

RolloutSpec defines the rollout strategy configuration for raw deployments
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**mode** | **str** | Mode specifies the rollout strategy mode. Valid values are \&quot;Availability\&quot; and \&quot;ResourceAware\&quot;. Availability mode: launches new pods first, then terminates old pods (maxUnavailable&#x3D;0, maxSurge&#x3D;ratio) ResourceAware mode: terminates old pods first, then launches new pods (maxSurge&#x3D;0, maxUnavailable&#x3D;ratio) | [default to '']
**ratio** | **str** | Ratio specifies the rollout ratio as a percentage (e.g., \&quot;25%\&quot;) or absolute number. This value is used to set either maxSurge (Availability mode) or maxUnavailable (ResourceAware mode). | [default to '']

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


