# V1alpha1KernelCacheCaptureStatus

KernelCacheCaptureStatus defines the observed state of KernelCacheCapture
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**captured_at** | [**V1Time**](V1Time.md) |  | [optional] 
**captured_cache_size_bytes** | **int** | CapturedCacheSizeBytes is the size of the captured cache in bytes | [optional] 
**conditions** | [**list[V1Condition]**](V1Condition.md) | Conditions represent the latest available observations | [optional] 
**detected_cache_path** | **str** | DetectedCachePath is the resolved cache path used for capture | [optional] 
**image_digest** | **str** | ImageDigest is the sha256 digest of the captured image | [optional] 
**kernel_cache_ref** | [**V1alpha1NamespacedName**](V1alpha1NamespacedName.md) |  | [optional] 
**phase** | **str** | Phase indicates current state of the capture | [optional] 
**pod_name** | **str** | PodName is the pod from which cache was captured | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


