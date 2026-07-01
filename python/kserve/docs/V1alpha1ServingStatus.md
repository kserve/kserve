# V1alpha1ServingStatus

ServingStatus tracks serving PVC usage (Phase 2)
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**namespace_counts** | [**dict(str, V1alpha1NamespaceServingCounts)**](V1alpha1NamespaceServingCounts.md) | Per-namespace breakdown (for debugging) | [optional] 
**total_namespaces** | **int** | Aggregate counts across all nodes/namespaces (Phase 2) | [default to 0]
**total_pods_ready** | **int** | Total pods using cache (any phase) | [default to 0]
**total_pods_terminating** | **int** | Total ready pods | [default to 0]
**total_pods_using** | **int** | Namespaces with serving PVCs | [default to 0]

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


