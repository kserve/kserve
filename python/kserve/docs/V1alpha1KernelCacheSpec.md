# V1alpha1KernelCacheSpec

KernelCacheSpec defines the desired state of KernelCache
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**access_modes** | **list[str]** | AccessModes for PV/PVC (optional, default: [ReadWriteMany] for Phase 1) | [optional] 
**framework** | **str** |  | [optional] 
**framework_version** | **str** |  | [optional] 
**gpu_type** | **str** | GPU metadata for automatic ISVC matching (Phase 2 webhook uses this) Populated from MCV GPU detection or sidecar auto-creation | [optional] 
**image** | **str** | Image is the OCI image URL containing kernel cache | [default to '']
**min_cuda_version** | **str** |  | [optional] 
**min_driver_version** | **str** |  | [optional] 
**pod_template** | [**V1alpha1KernelCachePodTemplate**](V1alpha1KernelCachePodTemplate.md) |  | [optional] 
**storage_class_name** | **str** | Phase 1 simple mode storage fields (removed in Phase 2 when NodeGroups added) StorageClassName for PV/PVC (optional, uses cluster default if unset) | [optional] 
**storage_size** | [**ResourceQuantity**](ResourceQuantity.md) |  | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


