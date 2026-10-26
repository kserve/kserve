# V1alpha1KernelCacheStatus

KernelCacheStatus defines the observed state of a KernelCache.
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**conditions** | [**list[V1Condition]**](V1Condition.md) | Conditions represent the latest availability observations. | [optional] 
**counts** | [**V1alpha1KernelCacheCounts**](V1alpha1KernelCacheCounts.md) |  | [optional] 
**mount_type** | **str** | MountType is the observed cache delivery mode. | [optional] 
**state** | **str** | State is the aggregate node preparation state. | [optional] 
**usage** | [**V1alpha1KernelCacheUsage**](V1alpha1KernelCacheUsage.md) |  | [optional] 
**verification** | [**V1alpha1KernelCacheVerificationStatus**](V1alpha1KernelCacheVerificationStatus.md) |  | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


