# V1alpha1KernelCacheNodeGroupStorage

KernelCacheNodeGroupStorage selects one of two mutually exclusive storage modes for the group:    - StorageClass mode:  dynamic provisioning through a named StorageClass.     Use this when the cluster has a working dynamic provisioner and the     operator should own volume lifecycle.   - Verbatim mode:      caller-supplied PersistentVolume and     PersistentVolumeClaim templates, mirroring the shape used by     LocalModelNodeGroup. Use this when the cluster does not have a suitable     dynamic provisioner and volumes must be pinned to individual nodes     (typically Local PVs).  Exactly one of StorageClass or PersistentVolumeSpec must be set. PersistentVolumeClaimSpec is required whenever PersistentVolumeSpec is set. hostPath volumes are rejected in verbatim mode; use Local PVs instead.
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**persistent_volume_claim_spec** | [**V1PersistentVolumeClaimSpec**](https://pkg.go.dev/k8s.io/api/core/v1#PersistentVolumeClaimSpec) |  | [optional]
**persistent_volume_spec** | [**V1PersistentVolumeSpec**](https://pkg.go.dev/k8s.io/api/core/v1#PersistentVolumeSpec) |  | [optional]
**storage_class** | [**V1alpha1KernelCacheNodeGroupStorageClass**](V1alpha1KernelCacheNodeGroupStorageClass.md) |  | [optional] 
**storage_limit** | [**ResourceQuantity**](https://pkg.go.dev/k8s.io/apimachinery/pkg/api/resource#Quantity) |  | [optional]

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


