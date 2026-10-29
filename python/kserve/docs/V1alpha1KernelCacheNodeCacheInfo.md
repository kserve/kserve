# V1alpha1KernelCacheNodeCacheInfo

KernelCacheNodeCacheInfo contains one cache's state on a node.
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**footprints** | [**V1alpha1KernelCacheFootprints**](V1alpha1KernelCacheFootprints.md) |  | 
**image_reference** | **str** | ImageReference identifies the artifact being prepared on this node. | [optional] 
**kernel_cache_ref** | [**V1alpha1NamespacedName**](V1alpha1NamespacedName.md) |  | 
**last_update** | [**V1Time**](V1Time.md) |  | [optional] 
**message** | **str** | Message provides details about the current preparation state. | [optional] 
**state** | **str** | State represents the preparation state of this cache on this node. | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


