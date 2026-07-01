# V1alpha1KernelCacheStatus

KernelCacheStatus defines the observed state of KernelCache
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**counts** | [**V1alpha1CacheCounts**](V1alpha1CacheCounts.md) |  | [optional] 
**gpu_compatibility** | [**V1alpha1GPUCompatibilitySummary**](V1alpha1GPUCompatibilitySummary.md) |  | [optional] 
**inference_services** | [**list[V1alpha1NamespacedName]**](V1alpha1NamespacedName.md) | Phase 2: ISVCs referencing this cache | [optional] 
**resolved_digest** | **str** | ResolvedDigest is the image digest (sha256:...) resolved by mutating webhook This field is immutable once set - copied from annotation on first reconcile Controller ALWAYS uses this field (not annotation) to prevent tampering | [optional] 
**serving_status** | [**V1alpha1ServingStatus**](V1alpha1ServingStatus.md) |  | [optional] 
**state** | **str** | State represents overall cache state across all nodes Hierarchy: Error &gt; Running &gt; Extracted &gt; Downloading &gt; Pending | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


