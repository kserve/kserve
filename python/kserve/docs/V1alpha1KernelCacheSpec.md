# V1alpha1KernelCacheSpec

KernelCacheSpec defines the desired state of KernelCache
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**access_modes** | **list[str]** | AccessModes for PV/PVC (only used when mountType&#x3D;pvc) | [optional] 
**image** | **str** | Image is the OCI image URL containing kernel cache | [default to '']
**image_pull_policy** | **str** | ImagePullPolicy for pulling the cache image specified in spec.image. For imageVolume mode: controls when Kubernetes pulls the image for volume mounting. For pvc mode: controls when the extraction job pulls the image to extract. | [optional] 
**mount_path** | **str** | MountPath in the container filesystem where the cache should be mounted. If empty (recommended), automatically computed from OCI image labels to maintain framework compatibility. The webhook determines the optimal mount path based on labels like io.kserve.km/cache-root-env.  Override only when automatic detection is insufficient. When set, SubPath within the volume (PVC or OCI image) is still auto-computed from labels.  Example: \&quot;/custom/cache/location\&quot; mounts the cache at this path instead of the label-derived path. | [optional] 
**mount_type** | **str** | MountType specifies how to mount the cache (pvc or imageVolume) | [optional] 
**pod_template** | [**V1alpha1KernelCachePodTemplate**](V1alpha1KernelCachePodTemplate.md) |  | [optional] 
**storage_class_name** | **str** | StorageClassName for PV/PVC (only used when mountType&#x3D;pvc) | [optional] 
**storage_size** | [**ResourceQuantity**](ResourceQuantity.md) |  | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


