# V1beta1KernelCacheRegistryAuth

KernelCacheRegistryAuth defines how KernelCache obtains registry credentials.
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**pull_role_ref** | [**V1beta1KernelCacheRegistryRoleRef**](V1beta1KernelCacheRegistryRoleRef.md) |  | [optional] 
**push_role_ref** | [**V1beta1KernelCacheRegistryRoleRef**](V1beta1KernelCacheRegistryRoleRef.md) |  | [optional] 
**token_ttl_seconds** | **int** | TokenTTLSeconds is the lifetime of a token issued through TokenRequest. The default is 600 seconds when serviceAccountToken is selected. | [optional] 
**type** | **str** | Type selects none or serviceAccountToken authentication. The zero value is treated as none. | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


