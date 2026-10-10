# V1alpha1KernelCacheCaptureSpec

KernelCacheCaptureSpec defines the desired capture configuration.
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**cache_paths** | [**list[V1alpha1KernelCachePath]**](V1alpha1KernelCachePath.md) | CachePaths overrides the cache paths passed to the capture sidecar. | [optional] 
**signing** | [**V1alpha1KernelCacheSigningSpec**](V1alpha1KernelCacheSigningSpec.md) |  | [optional] 
**source_ref** | [**V1alpha1KernelCacheSourceRef**](V1alpha1KernelCacheSourceRef.md) |  | 
**target_image** | **str** | TargetImage is the capture destination. If empty, the capture integration generates one. | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


