# V1alpha1ClusterStorageContainerList


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**api_version** | **str** | APIVersion defines the versioned schema of this representation of an object. Servers should convert recognized schemas to the latest internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources | [optional] 
**items** | [**List[V1alpha1ClusterStorageContainer]**](V1alpha1ClusterStorageContainer.md) |  | 
**kind** | **str** | Kind is a string value representing the REST resource this object represents. Servers may infer this from the endpoint the client submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds | [optional] 
**metadata** | [**V1ListMeta**](https://github.com/kubernetes-client/python/blob/master/kubernetes/docs/V1ListMeta.md) |  | [optional] 

## Example

```python
from kserve.models.v1alpha1_cluster_storage_container_list import V1alpha1ClusterStorageContainerList

# TODO update the JSON string below
json = "{}"
# create an instance of V1alpha1ClusterStorageContainerList from a JSON string
v1alpha1_cluster_storage_container_list_instance = V1alpha1ClusterStorageContainerList.from_json(json)
# print the JSON string representation of the object
print V1alpha1ClusterStorageContainerList.to_json()

# convert the object into a dict
v1alpha1_cluster_storage_container_list_dict = v1alpha1_cluster_storage_container_list_instance.to_dict()
# create an instance of V1alpha1ClusterStorageContainerList from a dict
v1alpha1_cluster_storage_container_list_form_dict = v1alpha1_cluster_storage_container_list.from_dict(v1alpha1_cluster_storage_container_list_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


