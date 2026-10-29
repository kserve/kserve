# V1alpha1KernelCachePath

KernelCachePath describes one cache directory in the source container and OCI artifact.
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**container_name** | **str** | ContainerName is the container that owns the cache directory. If omitted, KServe resolves the standard runtime container; set it for non-standard containers. | [optional] 
**container_path** | **str** | ContainerPath is the absolute path of the cache directory in the container. If omitted, KServe resolves VLLM_CACHE_ROOT and falls back to /root/.cache/vllm. | [optional] 
**oci_path** | **str** | OCIPath is the relative path of the cache directory in the OCI artifact. | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


