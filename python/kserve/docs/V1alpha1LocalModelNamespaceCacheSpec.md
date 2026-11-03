# V1alpha1LocalModelNamespaceCacheSpec

LocalModelNamespaceCacheSpec defines the spec for namespace-scoped local model cache.  Exactly one storage mode must be selected: either node-local caching via nodeGroups, or shared-PVC import via pvcRef. The two are mutually exclusive.
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**model_size** | [**ResourceQuantity**](ResourceQuantity.md) |  | 
**node_groups** | **list[str]** | group of nodes to cache the model on. Selects the legacy node-local caching mode. Mutually exclusive with pvcRef. | [optional] 
**pvc_ref** | **str** | PVCRef is the name of a pre-created PersistentVolumeClaim in the cache CR&#39;s namespace. Selects shared-PVC import mode: the model is imported once onto the referenced claim and shared read-only by serving replicas. The claim must be ReadWriteMany with filesystem volume mode. It is immutable; changing the destination requires a new cache CR. Mutually exclusive with nodeGroups. | [optional] 
**service_account_name** | **str** | ServiceAccountName specifies the service account to use for credential lookup. | [optional] 
**source_model_uri** | **str** | Original StorageUri | [default to '']
**storage** | [**V1alpha1LocalModelStorageSpec**](V1alpha1LocalModelStorageSpec.md) |  | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


