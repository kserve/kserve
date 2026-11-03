# V1alpha1KernelCacheUsage

KernelCacheUsage contains the Pods currently using a KernelCache.
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**pods** | [**list[V1alpha1KernelCachePodUsage]**](V1alpha1KernelCachePodUsage.md) | Pods lists active consumers. Entries are keyed by PodUID. | [optional] 
**total_pods_using** | **int** | TotalPodsUsing is derived from Pods. | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


