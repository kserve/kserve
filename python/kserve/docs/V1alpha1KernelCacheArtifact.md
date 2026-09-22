# V1alpha1KernelCacheArtifact

KernelCacheArtifact is the portable description of a completed OCI cache artifact. It is shared by KernelCacheCapture status and KernelCache spec.
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**cache_paths** | [**list[V1alpha1KernelCachePath]**](V1alpha1KernelCachePath.md) | CachePaths describes the cache directories represented in the artifact. | 
**identity** | [**V1alpha1KernelCacheIdentity**](V1alpha1KernelCacheIdentity.md) |  | 
**image_reference** | **str** | ImageReference is a full immutable OCI reference containing registry, repository, and digest. | [default to '']

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


