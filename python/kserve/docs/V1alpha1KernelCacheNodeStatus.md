# V1alpha1KernelCacheNodeStatus

KernelCacheNodeStatus defines per-node extraction and GPU compatibility status Agent owns all writes to this structure
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**cache_status** | [**dict(str, V1alpha1CacheNodeCacheInfo)**](V1alpha1CacheNodeCacheInfo.md) | CacheStatus maps cache name (unique within namespace) to full cache info and status Agent discovers caches by watching KernelCache CRs and populates this map Key format: \&quot;{namespace}/{name}\&quot; for uniqueness across namespaces | [optional] 
**counts** | [**V1alpha1NodeCacheCounts**](V1alpha1NodeCacheCounts.md) |  | [optional] 
**gpu_info** | [**list[V1alpha1GPUTypeInfo]**](V1alpha1GPUTypeInfo.md) | GPU info: list of GPU types detected on this node (from MCV) Can be empty (CPU-only node) or heterogeneous (mixed GPU types) | [optional] 
**node_name** | **str** | NodeName is the Kubernetes node this tracks | [default to '']

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


