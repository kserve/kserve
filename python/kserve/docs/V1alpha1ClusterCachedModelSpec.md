# V1alpha1ClusterCachedModelSpec

StorageContainerSpec defines the container spec for the storage initializer init container, and the protocols it supports.
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**cleanup_policy** | **str** |  | [default to '']
**model_size** | [**ResourceQuantity**](ResourceQuantity.md) |  | 
**node_group** | **str** |  | [default to '']
**pv_spec_name** | **str** |  | [default to '']
**pvc_spec_name** | **str** |  | [default to '']
**storage_type** | **str** | only local is supported for now | [default to '']
**storage_uri** | **str** |  | [default to '']

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


