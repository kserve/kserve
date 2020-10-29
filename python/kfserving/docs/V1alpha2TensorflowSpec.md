# V1alpha2TensorflowSpec

TensorflowSpec defines arguments for configuring Tensorflow model serving.
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**resources** | [**V1ResourceRequirements**](https://github.com/kubernetes-client/python/blob/master/kubernetes/docs/V1ResourceRequirements.md) | Defaults to requests and limits of 1CPU, 2Gb MEM. | [optional] 
**runtime_version** | **str** | TFServing docker image version(https://hub.docker.com/r/tensorflow/serving), default version can be set in the inferenceservice configmap. | [optional] 
**storage_uri** | **str** | The URI for the saved model(https://www.tensorflow.org/tutorials/keras/save_and_load) | 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


