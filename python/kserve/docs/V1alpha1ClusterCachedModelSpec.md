# V1alpha1ClusterCachedModelSpec

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**cleanup_policy** | **str** | Whether model cache controller creates a job to delete models on local disks. | [default to '']
**model_size** | [**ResourceQuantity**](ResourceQuantity.md) |  | 
**node_group** | **str** | A group of nodes to cache the model on. | [default to '']
**persistent_volume** | [**V1PersistentVolumeClaim**](V1PersistentVolumeClaim.md) |  | 
**persistent_volume_claim** | [**V1PersistentVolumeClaim**](V1PersistentVolumeClaim.md) |  | 
**storage_type** | **str** | only local is supported for now | [default to '']
**storage_uri** | **str** | Original StorageUri | [default to '']

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


