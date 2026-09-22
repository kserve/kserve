# V1alpha1KernelCacheNodeGroupStorageClass

KernelCacheNodeGroupStorageClass configures dynamic provisioning through a named StorageClass.
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**access_modes** | **list[str]** | AccessModes are the requested access modes for the provisioned volumes. | 
**name** | **str** | Name is the StorageClass name used to provision node-local volumes. | [default to '']
**size** | [**ResourceQuantity**](ResourceQuantity.md) |  | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


