# V1beta1StorageSpec


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**key** | **str** | The Storage Key in the secret for this model. | [optional] 
**parameters** | **Dict[str, str]** | Parameters to override the default storage credentials and config. | [optional] 
**path** | **str** | The path to the model object in the storage. It cannot co-exist with the storageURI. | [optional] 
**schema_path** | **str** | The path to the model schema file in the storage. | [optional] 

## Example

```python
from kserve.models.v1beta1_storage_spec import V1beta1StorageSpec

# TODO update the JSON string below
json = "{}"
# create an instance of V1beta1StorageSpec from a JSON string
v1beta1_storage_spec_instance = V1beta1StorageSpec.from_json(json)
# print the JSON string representation of the object
print V1beta1StorageSpec.to_json()

# convert the object into a dict
v1beta1_storage_spec_dict = v1beta1_storage_spec_instance.to_dict()
# create an instance of V1beta1StorageSpec from a dict
v1beta1_storage_spec_form_dict = v1beta1_storage_spec.from_dict(v1beta1_storage_spec_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


