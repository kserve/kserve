# V1beta1KernelCacheConfig

KernelCacheConfig contains the shared KernelCache configuration loaded from the kernelcache entry in the inferenceservice-config ConfigMap.
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**abandoned_capture_policy** | **str** |  | [optional] 
**artifact_security** | [**V1beta1KernelCacheArtifactSecurityConfig**](V1beta1KernelCacheArtifactSecurityConfig.md) |  | [optional] 
**default_mount_type** | **str** |  | [optional] 
**default_node_group** | **str** |  | [optional] 
**default_sidecar_injection** | **bool** |  | [default to False]
**enabled** | **bool** |  | [default to False]
**job_namespace** | **str** |  | [default to '']
**job_ttl_seconds_after_finished** | **int** |  | [optional] 
**mcv_capture_readiness_timeout_seconds** | **int** |  | [optional] 
**mcv_image** | **str** |  | [optional] 
**prefetch_image** | **str** |  | [optional] 
**reconcile_interval_seconds** | **int** |  | [optional] 
**registry** | [**V1beta1KernelCacheRegistryConfig**](V1beta1KernelCacheRegistryConfig.md) |  | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


