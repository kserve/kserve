# V1alpha1KernelCacheCaptureSpec

KernelCacheCaptureSpec defines the desired state of KernelCacheCapture
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**cache_framework** | **str** | CacheFramework identifies the inference framework running in the ISVC. The controller uses this to locate the framework&#39;s default cache directory and stamp the correct cache-type OCI label on the captured image (e.g. vllm → /root/.cache/vllm, gaudi → /home/kserve/.cache/habana). Exactly one of CacheFramework or CachePathOverride must be set. | [optional] 
**cache_path_override** | **str** | CachePathOverride specifies the exact directory path to capture when the framework does not write its cache to a standard location known to the controller. Use this when the ISVC is configured to write cache files to a custom path. The controller infers the cache type from the captured content rather than from a framework preset. Exactly one of CacheFramework or CachePathOverride must be set. | [optional] 
**target_image** | **str** | TargetImage is the OCI image URL where captured cache will be pushed | [default to '']

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


