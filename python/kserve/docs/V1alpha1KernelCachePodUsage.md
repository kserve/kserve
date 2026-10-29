# V1alpha1KernelCachePodUsage

KernelCachePodUsage identifies one Pod consuming a cache.
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**node_name** | **str** | NodeName is the node hosting the Pod. | [default to '']
**observed_at** | [**V1Time**](V1Time.md) |  | [optional] 
**pod_ref** | [**V1alpha1NamespacedName**](V1alpha1NamespacedName.md) |  | 
**pod_uid** | **str** | PodUID is the stable identity used as the list key. | [default to '']

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


