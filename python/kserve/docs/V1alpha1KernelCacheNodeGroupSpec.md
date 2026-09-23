# V1alpha1KernelCacheNodeGroupSpec

KernelCacheNodeGroupSpec defines node membership and workload tolerations for a KernelCacheNodeGroup.
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**node_selector** | **dict(str, str)** | NodeSelector selects the nodes that belong to this group. A node is a member of the group only when it carries every listed label. At least one label is required. | 
**tolerations** | [**list[V1Toleration]**](https://github.com/kubernetes-client/python/blob/master/kubernetes/docs/V1Toleration.md) | Tolerations are applied to controller-managed workloads that run on the selected nodes (for example, node-local preparation jobs). | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


