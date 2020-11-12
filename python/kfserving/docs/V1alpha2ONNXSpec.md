# V1alpha2ONNXSpec

ONNXSpec defines arguments for configuring ONNX model serving.
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**resources** | [**V1ResourceRequirements**](https://github.com/kubernetes-client/python/blob/master/kubernetes/docs/V1ResourceRequirements.md) | Defaults to requests and limits of 1CPU, 2Gb MEM. | [optional] 
**runtime_version** | **str** | ONNXRuntime docker image versions, default version can be set in the inferenceservice configmap | [optional] 
**storage_uri** | **str** | The URI of the exported onnx model(model.onnx) | 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


