# V1alpha1KernelCacheNodeGroupStorage

KernelCacheNodeGroupStorage selects one of two mutually exclusive storage modes for the group:    - StorageClass mode:  dynamic provisioning through a named StorageClass.     Use this when the cluster has a working dynamic provisioner and the     operator should own volume lifecycle.   - Verbatim mode:      caller-supplied PersistentVolume and     PersistentVolumeClaim templates, mirroring the shape used by     LocalModelNodeGroup. Use this when the cluster does not have a suitable     dynamic provisioner and volumes must be pinned to individual nodes     (typically Local PVs).  Exactly one of StorageClass or PersistentVolumeSpec must be set. PersistentVolumeClaimSpec is required whenever PersistentVolumeSpec is set. hostPath volumes are rejected in verbatim mode; use Local PVs instead.
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**persistent_volume_claim_spec** | [**V1PersistentVolumeClaimSpec**](V1PersistentVolumeClaimSpec.md) |  | [optional] 
**persistent_volume_spec** | [**V1PersistentVolumeSpec**](V1PersistentVolumeSpec.md) |  | [optional] 
**storage_class** | [**V1alpha1KernelCacheNodeGroupStorageClass**](V1alpha1KernelCacheNodeGroupStorageClass.md) |  | [optional] 
**storage_limit** | [**ResourceQuantity**](ResourceQuantity.md) |  | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


