# V1alpha1KernelCacheSpec

KernelCacheSpec defines the desired state of KernelCache
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**image** | **str** | Image is the OCI image URL containing the kernel cache. Both tags and digests are accepted (e.g. myrepo/cache:v1 or myrepo/cache@sha256:abc123). The webhook resolves tags to digests and pins the resolved digest in status to prevent cache drift. | [default to '']
**image_pull_policy** | **str** | ImagePullPolicy controls when the cache image is pulled. For pvc mode: governs when the extraction job pulls the image. For imageVolume mode: governs when Kubernetes pulls the image for volume mounting. | [optional] 
**mount_path** | **str** | MountPath in the container filesystem where the cache is mounted. When empty (recommended), auto-computed from OCI image labels to maintain framework compatibility. Override only when automatic detection is insufficient. | [optional] 
**mount_type** | **str** | MountType specifies how the cache image is delivered to serving containers. pvc: extracts the OCI image into a PersistentVolumeClaim; requires spec.pvc. imageVolume: mounts the OCI image directly without extraction (Kubernetes 1.33+). Immutable after creation. | [optional] 
**pvc** | [**V1alpha1KernelCachePVCConfig**](V1alpha1KernelCachePVCConfig.md) |  | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


