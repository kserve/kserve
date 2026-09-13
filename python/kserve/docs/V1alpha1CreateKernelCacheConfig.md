# V1alpha1CreateKernelCacheConfig

CreateKernelCacheConfig controls auto-creation of KernelCache after successful capture
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**enabled** | **bool** | Enabled controls whether to auto-create KernelCache | [optional] 
**mount_type** | **str** | MountType for the auto-created KernelCache | [optional] 
**name** | **str** | Name for the auto-created KernelCache (defaults to KCC name) | [optional] 
**pull_secret_ref** | [**V1LocalObjectReference**](https://github.com/kubernetes-client/python/blob/master/kubernetes/docs/V1LocalObjectReference.md) |  | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


