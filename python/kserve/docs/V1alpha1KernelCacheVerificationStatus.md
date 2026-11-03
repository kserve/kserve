# V1alpha1KernelCacheVerificationStatus

KernelCacheVerificationStatus contains artifact verification results.
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**message** | **str** | Message provides additional verification details. | [optional] 
**mode** | **str** | Mode is the configured verification mode. | [default to '']
**reason** | **str** | Reason describes the verification result. | [optional] 
**state** | **str** | State is the latest verification outcome. | [default to '']
**verified** | **bool** | Verified indicates whether the immutable artifact passed verification. | [default to False]
**verified_at** | [**V1Time**](V1Time.md) |  | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


