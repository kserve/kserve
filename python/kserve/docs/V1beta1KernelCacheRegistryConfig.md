# V1beta1KernelCacheRegistryConfig

KernelCacheRegistryConfig defines the registry endpoint, trust bundle, and authentication used by KernelCache capture and prefetch operations.
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**auth** | [**V1beta1KernelCacheRegistryAuth**](V1beta1KernelCacheRegistryAuth.md) |  | [optional] 
**ca_config_map_ref** | [**V1beta1KernelCacheConfigMapKeyRef**](V1beta1KernelCacheConfigMapKeyRef.md) |  | [optional] 
**endpoint** | **str** | Endpoint is the OCI registry host and optional port. | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


