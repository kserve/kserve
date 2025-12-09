# V1alpha1LocalModelCacheSpec

LocalModelCacheSpec
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**model_size** | [**ResourceQuantity**](ResourceQuantity.md) |  | 
**node_groups** | **list[str]** | group of nodes to cache the model on. Todo: support more than 1 node groups | 
**service_account_name** | **str** | ServiceAccountName specifies the service account to use for credential lookup. The service account should have secrets attached that contain the credentials for accessing the model storage (e.g., HuggingFace token, S3 credentials). | [optional] 
**source_model_uri** | **str** | Original StorageUri | [default to '']
**storage** | [**V1alpha1LocalModelStorageSpec**](V1alpha1LocalModelStorageSpec.md) |  | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


