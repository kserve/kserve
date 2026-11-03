# V1alpha1KernelCacheSigningStatus

KernelCacheSigningStatus contains artifact signing results.
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**message** | **str** | Message provides additional signing details. | [optional] 
**mode** | **str** | Mode is the configured signing mode. | [default to '']
**reason** | **str** | Reason describes the signing result. | [optional] 
**signed** | **bool** | Signed indicates whether a signature was created. | [default to False]
**signed_at** | [**V1Time**](V1Time.md) |  | [optional] 
**state** | **str** | State is the latest signing outcome. | [default to '']

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


