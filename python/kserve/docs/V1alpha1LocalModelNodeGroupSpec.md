# V1alpha1LocalModelNodeGroupSpec

LocalModelNodeGroupSpec defines a group of nodes for to download the model to.
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**job_tolerations** | [**list[V1Toleration]**](https://github.com/kubernetes-client/python/blob/master/kubernetes/docs/V1Toleration.md) | Used to specify tolerations for download jobs | [optional] 
**persistent_volume_claim_spec** | [**V1PersistentVolumeClaimSpec**](V1PersistentVolumeClaimSpec.md) |  | 
**persistent_volume_spec** | [**V1PersistentVolumeSpec**](V1PersistentVolumeSpec.md) |  | 
**storage_limit** | [**ResourceQuantity**](ResourceQuantity.md) |  | 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


