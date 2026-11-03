# V1alpha1KernelCacheSpec

KernelCacheSpec defines the desired artifact and node preparation target.
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**artifact** | [**V1alpha1KernelCacheArtifact**](V1alpha1KernelCacheArtifact.md) |  | 
**mount_type** | **str** | MountType selects the cache delivery mode. OCI is the only supported mode. | [optional] 
**node_group_ref** | [**V1LocalObjectReference**](https://github.com/kubernetes-client/python/blob/master/kubernetes/docs/V1LocalObjectReference.md) |  | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


