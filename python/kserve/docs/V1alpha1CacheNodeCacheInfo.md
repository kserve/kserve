# V1alpha1CacheNodeCacheInfo

CacheNodeCacheInfo tracks full cache information and status on one node Combines cache identity (name/namespace/image/digest) with extraction state Agent populates all fields by watching KernelCache CRs and tracking extraction jobs
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**compatible_gp_us** | **list[int]** | GPU compatibility (from MCV validation during extraction) Lists GPU IDs that are compatible/incompatible with this cache | [optional] 
**digest** | **str** |  | [optional] 
**image** | **str** |  | [default to '']
**incompatible_gp_us** | **list[int]** |  | [optional] 
**last_update** | [**V1Time**](V1Time.md) |  | [optional] 
**message** | **str** | Message provides details about current state (e.g., error messages) | [optional] 
**name** | **str** | Cache identity (agent reads from KernelCache CR) | [default to '']
**namespace** | **str** |  | [default to '']
**serving_namespaces** | [**dict(str, V1alpha1NamespaceServingCounts)**](V1alpha1NamespaceServingCounts.md) | Serving PVC usage on this node (per-namespace counts) | [optional] 
**state** | **str** | State represents this cache&#39;s state on this specific node | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


