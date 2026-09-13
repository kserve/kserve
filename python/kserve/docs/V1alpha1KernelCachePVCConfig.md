# V1alpha1KernelCachePVCConfig

KernelCachePVCConfig holds PVC-specific configuration for cache extraction and serving. Only applies when mountType is pvc.  Provisioning modes are mutually exclusive:   - Dynamic (storageClassName): the StorageClass provisioner creates the PV automatically.   - Static (volumeName): binds to a pre-existing PV by name.  The storage fields (storageClassName, volumeName, storageSize, accessModes) are immutable after the PVC is provisioned. Changing them after creation conflicts with the existing PVC and has undefined behavior.
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**access_modes** | **list[str]** | AccessModes for the PVC. When unset, the StorageClass default applies. Multi-node serving requires ReadWriteMany (RWX) so the extraction job and ISVC pods on different nodes can all mount the PVC concurrently; ensure the StorageClass supports RWX before setting it. Single-node deployments can use ReadWriteOnce (RWO). | [optional] 
**pod_template** | [**V1alpha1KernelCachePodTemplate**](V1alpha1KernelCachePodTemplate.md) |  | [optional] 
**storage_class_name** | **str** | StorageClassName names the StorageClass used for dynamic PV provisioning. The StorageClass provisioner creates a PV automatically when the PVC is created. Omit to use the cluster&#39;s default StorageClass. Mutually exclusive with volumeName. | [optional] 
**storage_size** | [**ResourceQuantity**](ResourceQuantity.md) |  | [optional] 
**volume_name** | **str** | VolumeName binds the PVC to a pre-existing PV by name (static provisioning). Use when you have pre-created a PV with specific storage type or topology. storageSize is ignored when volumeName is set (capacity is defined on the PV). Mutually exclusive with storageClassName. | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


