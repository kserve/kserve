# V1beta1RolloutSpec

RolloutSpec defines the rollout strategy configuration using Kubernetes deployment strategy
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**max_surge** | **str** | MaxSurge specifies the maximum number of pods that can be created above the desired replica count. Can be an absolute number (ex: 5) or a percentage of desired pods (ex: 10%). | [default to '']
**max_unavailable** | **str** | MaxUnavailable specifies the maximum number of pods that can be unavailable during the update. Can be an absolute number (ex: 5) or a percentage of desired pods (ex: 10%). | [default to '']

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


