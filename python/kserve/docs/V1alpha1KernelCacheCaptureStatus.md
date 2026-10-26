# V1alpha1KernelCacheCaptureStatus

KernelCacheCaptureStatus defines the observed capture result.
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**active_session** | [**V1alpha1KernelCacheCaptureSession**](V1alpha1KernelCacheCaptureSession.md) |  | [optional] 
**artifact** | [**V1alpha1KernelCacheArtifact**](V1alpha1KernelCacheArtifact.md) |  | [optional] 
**captured_at** | [**V1Time**](V1Time.md) |  | [optional] 
**captured_cache_size_bytes** | **int** | CapturedCacheSizeBytes is the captured cache size. | [optional] 
**conditions** | [**list[V1Condition]**](V1Condition.md) | Conditions report whether the capture is ready and why it is not ready. | [optional] 
**kernel_cache_ref** | [**V1alpha1NamespacedName**](V1alpha1NamespacedName.md) |  | [optional] 
**phase** | **str** | Phase indicates the current capture phase. | [optional] 
**runtime_result** | **dict(str, str)** | RuntimeResult contains the raw key-value result reported by the capture sidecar. | [optional] 
**signing** | [**V1alpha1KernelCacheSigningStatus**](V1alpha1KernelCacheSigningStatus.md) |  | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


