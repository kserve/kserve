# V1alpha2PyTorchSpec

PyTorchSpec defines arguments for configuring PyTorch model serving.
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**model_class_name** | **str** | Defaults PyTorch model class name to &#39;PyTorchModel&#39; | [optional] 
**resources** | [**V1ResourceRequirements**](https://github.com/kubernetes-client/python/blob/master/kubernetes/docs/V1ResourceRequirements.md) | Defaults to requests and limits of 1CPU, 2Gb MEM. | [optional] 
**runtime_version** | **str** | PyTorch KFServer docker image version which defaults to latest release | [optional] 
**storage_uri** | **str** | The URI of the trained model which contains model.pt | 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


