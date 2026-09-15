# V1alpha1LocalModelCacheSpec

LocalModelCacheSpec
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**image_pull_secrets** | [**list[V1LocalObjectReference]**](https://github.com/kubernetes-client/python/blob/master/kubernetes/docs/V1LocalObjectReference.md) | ImagePullSecrets are kubernetes.io/dockerconfigjson secrets in the download job namespace used to authenticate OCI (oci://) imports. Only the first secret is projected into the download container; merge multiple registries into one secret. Do not use serviceAccountName for dockerconfigjson — the shared credential dispatcher has no OCI/oras branch. | [optional] 
**model_size** | [**ResourceQuantity**](ResourceQuantity.md) |  | 
**node_groups** | **list[str]** | group of nodes to cache the model on. Todo: support more than 1 node groups | 
**service_account_name** | **str** | ServiceAccountName specifies the service account to use for credential lookup. The service account must exist in the download job namespace (localModel.jobNamespace). | [optional] 
**source_model_uri** | **str** | Original StorageUri | [default to '']
**storage** | [**V1alpha1LocalModelStorageSpec**](V1alpha1LocalModelStorageSpec.md) |  | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


