# V1alpha1KernelCacheCaptureSpec

KernelCacheCaptureSpec defines the desired state of KernelCacheCapture
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**cache_path** | **str** | CachePath specifies an explicit cache directory path Overrides CachePreset if both are specified | [optional] 
**cache_preset** | **str** | CachePreset specifies a known cache location preset (vllm, tgi, triton-python) Mutually exclusive with CachePath | [optional] 
**create_kernel_cache** | [**V1alpha1CreateKernelCacheConfig**](V1alpha1CreateKernelCacheConfig.md) |  | [optional] 
**registry_secret_ref** | [**V1alpha1SecretKeySelector**](V1alpha1SecretKeySelector.md) |  | [optional] 
**target_image** | **str** | TargetImage is the OCI image URL where captured cache will be pushed | [default to '']
**trigger** | **bool** | Trigger initiates the cache capture when set to true | [optional] 
**volume_strategy** | **str** | VolumeStrategy specifies how to access the cache shared: inject shared emptyDir volume (default) copy: use kubectl cp to copy cache from main container | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


