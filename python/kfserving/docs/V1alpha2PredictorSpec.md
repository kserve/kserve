# V1alpha2PredictorSpec

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**custom** | [**V1alpha2CustomSpec**](V1alpha2CustomSpec.md) | Spec for a custom predictor | [optional] 
**logger** | [**V1alpha2Logger**](V1alpha2Logger.md) | Activate request/response logging | [optional] 
**max_replicas** | **int** | This is the up bound for autoscaler to scale to | [optional] 
**min_replicas** | **int** | Minimum number of replicas, pods won&#39;t scale down to 0 in case of no traffic | [optional] 
**onnx** | [**V1alpha2ONNXSpec**](V1alpha2ONNXSpec.md) | Spec for ONNX runtime (https://github.com/microsoft/onnxruntime) | [optional] 
**parallelism** | **int** | Parallelism specifies how many requests can be processed concurrently, this sets the target concurrency for Autoscaling(KPA). For model servers that support tuning parallelism will use this value, by default the parallelism is the number of the CPU cores for most of the model servers. | [optional] 
**pytorch** | [**V1alpha2PyTorchSpec**](V1alpha2PyTorchSpec.md) | Spec for PyTorch predictor | [optional] 
**service_account_name** | **str** | ServiceAccountName is the name of the ServiceAccount to use to run the service | [optional] 
**sklearn** | [**V1alpha2SKLearnSpec**](V1alpha2SKLearnSpec.md) | Spec for SKLearn predictor | [optional] 
**tensorflow** | [**V1alpha2TensorflowSpec**](V1alpha2TensorflowSpec.md) | Spec for Tensorflow Serving (https://github.com/tensorflow/serving) | [optional] 
**triton** | [**V1alpha2TritonSpec**](V1alpha2TritonSpec.md) | Spec for Triton Inference Server (https://github.com/NVIDIA/triton-inference-server) | [optional] 
**xgboost** | [**V1alpha2XGBoostSpec**](V1alpha2XGBoostSpec.md) | Spec for XGBoost predictor | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


