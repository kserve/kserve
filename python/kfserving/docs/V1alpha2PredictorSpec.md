# V1alpha2PredictorSpec

PredictorSpec defines the configuration for a predictor, The following fields follow a \"1-of\" semantic. Users must specify exactly one spec.## Properties
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**batcher** | [**V1alpha2Batcher**](V1alpha2Batcher.md) | Activate request batching | [optional] 
**custom** | [**V1alpha2CustomSpec**](V1alpha2CustomSpec.md) | Spec for a custom predictor | [optional] 
**lightgbm** | [**V1alpha2LightGBMSpec**](V1alpha2LightGBMSpec.md) | Spec for LightGBM predictor | [optional] 
**logger** | [**V1alpha2Logger**](V1alpha2Logger.md) | Activate request/response logging | [optional] 
**max_replicas** | **int** | This is the up bound for autoscaler to scale to | [optional] 
**min_replicas** | **int** | Minimum number of replicas which defaults to 1, when minReplicas &#x3D; 0 pods scale down to 0 in case of no traffic | [optional] 
**onnx** | [**V1alpha2ONNXSpec**](V1alpha2ONNXSpec.md) | Spec for ONNX runtime (https://github.com/microsoft/onnxruntime) | [optional] 
**parallelism** | **int** | Parallelism specifies how many requests can be processed concurrently, this sets the hard limit of the container concurrency(https://knative.dev/docs/serving/autoscaling/concurrency). | [optional] 
**pmml** | [**V1alpha2PMMLSpec**](V1alpha2PMMLSpec.md) | Spec for PMML predictor | [optional] 
**pytorch** | [**V1alpha2PyTorchSpec**](V1alpha2PyTorchSpec.md) | Spec for PyTorch predictor | [optional] 
**service_account_name** | **str** | ServiceAccountName is the name of the ServiceAccount to use to run the service | [optional] 
**sklearn** | [**V1alpha2SKLearnSpec**](V1alpha2SKLearnSpec.md) | Spec for SKLearn predictor | [optional] 
**tensorflow** | [**V1alpha2TensorflowSpec**](V1alpha2TensorflowSpec.md) | Spec for Tensorflow Serving (https://github.com/tensorflow/serving) | [optional] 
**triton** | [**V1alpha2TritonSpec**](V1alpha2TritonSpec.md) | Spec for Triton Inference Server (https://github.com/triton-inference-server/server) | [optional] 
**xgboost** | [**V1alpha2XGBoostSpec**](V1alpha2XGBoostSpec.md) | Spec for XGBoost predictor | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)
