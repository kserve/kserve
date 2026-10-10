# V1alpha1KernelCacheFootprints

KernelCacheFootprints contains the stable identities used to identify a cache artifact.
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**compatibility_footprint** | **str** | CompatibilityFootprint is the SHA-256 identity of the compatibility factors. It is omitted for latest or implicitly-latest runtime images. | [optional] 
**workload_footprint** | **str** | WorkloadFootprint is the SHA-256 identity of the cache-relevant workload. It is omitted when the declared runtime image is not digest-pinned. | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


