# V1alpha1StorageContainerSpec

StorageContainerSpec defines the container spec for the storage initializer init container, and the protocols it supports.

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**container** | [**V1Container**](https://github.com/kubernetes-client/python/blob/master/kubernetes/docs/V1Container.md) |  | 
**supported_uri_formats** | [**List[V1alpha1SupportedUriFormat]**](V1alpha1SupportedUriFormat.md) | List of URI formats that this container supports | 

## Example

```python
from kserve.models.v1alpha1_storage_container_spec import V1alpha1StorageContainerSpec

# TODO update the JSON string below
json = "{}"
# create an instance of V1alpha1StorageContainerSpec from a JSON string
v1alpha1_storage_container_spec_instance = V1alpha1StorageContainerSpec.from_json(json)
# print the JSON string representation of the object
print V1alpha1StorageContainerSpec.to_json()

# convert the object into a dict
v1alpha1_storage_container_spec_dict = v1alpha1_storage_container_spec_instance.to_dict()
# create an instance of V1alpha1StorageContainerSpec from a dict
v1alpha1_storage_container_spec_form_dict = v1alpha1_storage_container_spec.from_dict(v1alpha1_storage_container_spec_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


