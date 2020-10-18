# V1alpha2TensorflowSpec

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**resources** | [**V1ResourceRequirements**](V1ResourceRequirements.md) | Defaults to requests and limits of 1CPU, 2Gb MEM. | [optional] 
**runtime_version** | **str** | TFServing docker image version(https://hub.docker.com/r/tensorflow/serving), default version can be set in the inferenceservice configmap. | [optional] 
**storage_uri** | **str** | The URI for the saved model(https://www.tensorflow.org/tutorials/keras/save_and_load) | 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


