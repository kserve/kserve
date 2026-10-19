# V1alpha1LocalModelNamespaceCacheSpec

LocalModelNamespaceCacheSpec defines the spec for namespace-scoped local model cache.  Exactly one storage mode must be selected: either node-local caching via nodeGroups, or shared-PVC import via pvcRef. The two are mutually exclusive.
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**image_pull_secrets** | [**list[V1LocalObjectReference]**](https://github.com/kubernetes-client/python/blob/master/kubernetes/docs/V1LocalObjectReference.md) | ImagePullSecrets is a single kubernetes.io/dockerconfigjson secret used to authenticate OCI (oci://) imports. The list shape matches PodSpec.imagePullSecrets; MaxItems&#x3D;1 because credential merging is not supported. Combine credentials for multiple registries into one secret. For nodeGroups caches the named Secret must exist in the download job namespace (localModel.jobNamespace); for pvcRef caches it must exist in this cache&#39;s namespace, where the import Job runs. Credentials from serviceAccountName and storage are not used for oci:// sources.  Trust boundary: a namespaced nodeGroups cache can name any dockerconfigjson Secret already present in the shared job namespace by that Secret&#39;s metadata.name. There is no extra ownership check; knowing the name is enough. Secrets are not copied from the cache namespace. Prefer pvcRef (import Job in the cache namespace) when tenants must not share the cluster job-namespace credential store. | [optional] 
**model_size** | [**ResourceQuantity**](ResourceQuantity.md) |  | 
**node_groups** | **list[str]** | group of nodes to cache the model on. Selects the legacy node-local caching mode. Mutually exclusive with pvcRef. | [optional] 
**pvc_ref** | **str** | PVCRef is the name of a pre-created PersistentVolumeClaim in the cache CR&#39;s namespace. Selects shared-PVC import mode: the model is imported once onto the referenced claim and shared read-only by serving replicas. The claim must be ReadWriteMany with filesystem volume mode. It is immutable; changing the destination requires a new cache CR. Mutually exclusive with nodeGroups. | [optional] 
**service_account_name** | **str** | ServiceAccountName specifies the service account to use for credential lookup. For nodeGroups caches it must exist in the download job namespace (localModel.jobNamespace); for pvcRef caches it must exist in this cache&#39;s namespace, where the import Job runs. | [optional] 
**source_model_uri** | **str** | Original StorageUri | [default to '']
**storage** | [**V1alpha1LocalModelStorageSpec**](V1alpha1LocalModelStorageSpec.md) |  | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


