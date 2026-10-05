# V1alpha1LocalModelCacheSpec

LocalModelCacheSpec
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**image_pull_secrets** | [**list[V1LocalObjectReference]**](https://github.com/kubernetes-client/python/blob/master/kubernetes/docs/V1LocalObjectReference.md) | ImagePullSecrets is a single kubernetes.io/dockerconfigjson secret in the download job namespace used to authenticate OCI (oci://) imports. The list shape matches PodSpec.imagePullSecrets; MaxItems=1 because credential merging is not supported. Combine credentials for multiple registries into one secret. Credentials from serviceAccountName and storage are not used for oci:// sources. | [optional] 
**model_size** | [**ResourceQuantity**](ResourceQuantity.md) |  | 
**node_groups** | **list[str]** | group of nodes to cache the model on. Todo: support more than 1 node groups | 
**service_account_name** | **str** | ServiceAccountName specifies the service account to use for credential lookup. The service account must exist in the download job namespace (localModel.jobNamespace). | [optional] 
**source_model_uri** | **str** | Original StorageUri | [default to '']
**storage** | [**V1alpha1LocalModelStorageSpec**](V1alpha1LocalModelStorageSpec.md) |  | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


