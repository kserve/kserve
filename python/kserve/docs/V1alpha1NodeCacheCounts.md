# V1alpha1NodeCacheCounts

NodeCacheCounts aggregates cache and pod counts across all caches on a node
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**caches_error** | **int** | CachesError - caches in Error state | [default to 0]
**caches_in_use** | **int** | CachesInUse - caches in Running state (mounted by pods) | [default to 0]
**caches_not_in_use** | **int** | CachesNotInUse - caches in Extracted state (available but not mounted) | [default to 0]
**total_pods_terminating** | **int** | TotalPodsTerminating - total pods terminating | [default to 0]
**total_pods_using** | **int** | TotalPodsUsing - total pods using any cache on this node (any phase) | [default to 0]

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


