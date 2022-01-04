## KServe Features and Examples

### Deploy InferenceService with Predictor
KServe provides a simple Kubernetes CRD to allow deploying single or multiple trained models onto model servers such as [TFServing](https://www.tensorflow.org/tfx/guide/serving), 
[TorchServe](https://pytorch.org/serve/server.html), [ONNXRuntime](https://github.com/microsoft/onnxruntime), [Triton Inference Server](https://docs.nvidia.com/deeplearning/triton-inference-server/user-guide/docs).
In addition [KFServer](https://github.com/kserve/kserve/tree/master/python/kserve) is the Python model server implemented in KServe itself prediction v1 protocol,
[MLServer](https://github.com/SeldonIO/MLServer) implements the [prediction v2 protocol](https://github.com/kserve/kserve/tree/master/docs/predict-api/v2) with both REST and gRPC.
These model servers are able to provide out-of-the-box model serving, but you could also choose to build your own model server for more complex use case.
KServe provides basic API primitives to allow you easily build custom model server, you can use other tools like [BentoML](https://docs.bentoml.org/en/latest) to build your custom model serve image.

After models are deployed onto model servers with KServe, you get all the following serverless features provided by KServe.
- Scale to and from Zero
- Request based Autoscaling on CPU/GPU
- Revision Management
- Optimized Container
- Batching
- Request/Response logging
- Scalable Multi Model Serving
- Traffic management
- Security with AuthN/AuthZ
- Distributed Tracing
- Out-of-the-box metrics
- Ingress/Egress control

| Out-of-the-box Predictor  | Exported model| Prediction Protocol | HTTP | gRPC | Versions| Examples |
| ------------- | ------------- | ------------- | ------------- | ------------- | ------------- | ------------- |
| [Triton Inference Server](https://github.com/triton-inference-server/server) | [TensorFlow,TorchScript,ONNX,TensorRT](https://docs.nvidia.com/deeplearning/triton-inference-server/user-guide/docs/model_repository.html)| v2 | :heavy_check_mark: | :heavy_check_mark: | [Compatibility Matrix](https://docs.nvidia.com/deeplearning/frameworks/support-matrix/index.html)| [Triton Examples](./v1beta1/triton) |
| [TFServing](https://www.tensorflow.org/tfx/guide/serving) | [TensorFlow SavedModel](https://www.tensorflow.org/guide/saved_model) | v1 | :heavy_check_mark: | :heavy_check_mark: | [TFServing Versions](https://github.com/tensorflow/serving/releases) | [TensorFlow Examples](./v1beta1/tensorflow)  |
| [TorchServe](https://pytorch.org/serve/server.html) | [Eager Model/TorchScript](https://pytorch.org/docs/master/generated/torch.save.html) | v1/v2 | :heavy_check_mark: | :heavy_check_mark: | 0.4.1 | [TorchServe Examples](./v1beta1/torchserve)  |
| [TorchServe Native](https://pytorch.org/serve/server.html) | [Eager Model/TorchScript](https://pytorch.org/docs/master/generated/torch.save.html) | native | :heavy_check_mark: | :heavy_check_mark: | 0.4.1 | [TorchServe Examples](./v1beta1/custom/torchserve)  |
| [ONNXRuntime](https://github.com/microsoft/onnxruntime)  | [Exported ONNX Model](https://github.com/onnx/tutorials#converting-to-onnx-format) | v1 | :heavy_check_mark: | :heavy_check_mark: | [Compatibility](https://github.com/microsoft/onnxruntime#compatibility) |[ONNX Style Model](./v1beta1/onnx)  |
| [SKLearn MLServer](https://github.com/SeldonIO/MLServer) | [Pickled Model](https://scikit-learn.org/stable/modules/model_persistence.html) | v2 | :heavy_check_mark: | :heavy_check_mark: | 0.23.1 | [SKLearn Iris V2](./v1beta1/sklearn/v2)  |
| [XGBoost MLServer](https://github.com/SeldonIO/MLServer) | [Saved Model](https://xgboost.readthedocs.io/en/latest/tutorials/saving_model.html) | v2 | :heavy_check_mark: | :heavy_check_mark: | 1.1.1 | [XGBoost Iris V2](./v1beta1/xgboost)  |
| [SKLearn KFServer](https://github.com/kserve/kserve/tree/master/python/sklearnserver) | [Pickled Model](https://scikit-learn.org/stable/modules/model_persistence.html) | v1 | :heavy_check_mark: | -- | 0.20.3 | [SKLearn Iris](./v1beta1/sklearn/v1)  |
| [PMML KFServer](https://github.com/kserve/kserve/tree/master/python/pmmlserver) | [PMML](http://dmg.org/pmml/v4-4-1/GeneralStructure.html) | v1 | :heavy_check_mark: | -- | [PMML4.4.1](https://github.com/autodeployai/pypmml) | [SKLearn PMML](./v1beta1/pmml)  |
| [LightGBM KFServer](https://github.com/kserve/kserve/tree/master/python/lightgbm) | [Saved LightGBM Model](https://lightgbm.readthedocs.io/en/latest/pythonapi/lightgbm.Booster.html#lightgbm.Booster.save_model) | v1 | :heavy_check_mark: | -- | 2.3.1 | [LightGBM Iris](./v1beta1/lightgbm)  |

| Custom Predictor  | Examples |
| ------------- |  ------------- |
| Deploy model on custom KFServer | [Custom KFServer](./v1beta1/custom/custom_model)|
| Deploy model on BentoML | [SKLearn Iris with BentoML](./bentoml)|
| Deploy model on custom HTTP Server  | [Prebuilt Model Server](./v1beta1/custom/prebuilt-image)|

In addition to deploy InferenceService with HTTP/gRPC endpoint, you can also deploy InferenceService with [Knative Event Sources](https://knative.dev/docs/eventing/sources/index.html) such as Kafka
, you can find an example [here](./kafka) which shows how to build an async inference pipeline. 

### Deploy InferenceService with Transformer
KServe transformer enables users to define a pre/post processing step before the prediction and explanation workflow.
Using the preprocessing step, users can also enrich the inference request with features retrieved from a feature store.
KServe transformer runs as a separate microservice and can work with any type of pre-packaged model server, it can also 
scale differently from the predictor if your transformer is CPU bound while predictor requires running on GPU. 

| Features  | Examples |
| ------------- | ------------- |
| Deploy Transformer with Feast | [Get online features from Feast](./v1beta1/transformer/feast)  |
| Deploy Transformer with Triton Server | [BERT Model with tokenizer](./v1beta1/triton/bert)  |
| Deploy Transformer with TorchServe| [Image classifier](./v1beta1/transformer/torchserve_image_transformer)  |
| Deploy Transformer with TorchScript model| [Image classifier](./v1beta1/triton/torchscript)  |

### Deploy InferenceService with Explainer
Model explainability answers the question: "Why did my model make this prediction" for a given instance. KServe 
integrates with [Alibi Explainer](https://github.com/SeldonIO/alibi) which implements a black-box algorithm by generating a lot of similar looking instances 
for a given instance and send out to the model server to produce an explanation.

Additionally KServe also integrates with The [AI Explainability 360 (AIX360)](https://ai-explainability-360.org/) toolkit, an LF AI Foundation incubation project, which is an open-source library that supports the interpretability and explainability of datasets and machine learning models. The AI Explainability 360 Python package includes a comprehensive set of algorithms that cover different dimensions of explanations along with proxy explainability metrics. In addition to native algorithms, AIX360 also provides algorithms from LIME and Shap.

| Features  | Examples |
| ------------- | ------------- |
| Deploy Alibi Image Explainer| [CIFAR10 Image Explainer](./explanation/alibi/cifar10)  |
| Deploy Alibi Income Explainer| [Income Explainer](./explanation/alibi/income)  |
| Deploy Alibi Text Explainer| [Alibi Text Explainer](./explanation/alibi/moviesentiment) |
| Deploy AIX360 Image Explainer| [AIX360 Image Explainer](./explanation/aix/mnist/README.md) |

### Deploy InferenceService with Multiple Models(Alpha)
Multi Model Serving allows deploying `TrainedModels` at scale without being bounded by the Kubernetes compute resources(CPU/GPU/Memory), 
service/pod limits and reducing the TCO, see [Multi Model Serving](../MULTIMODELSERVING_GUIDE.md) for more details. 
Multi Model Serving is supported for Triton, SKLearn/XGBoost as well as Custom KFServer.
 
| Features  | Examples |
| ------------- | ------------- |
| Deploy multiple models with Triton Inference Server| [Multi Model Triton InferenceService](./multimodelserving/triton/README.md)  |
| Deploy multiple models with SKLearn/XGBoost KFServer| [Multi Model SKLearn InferenceService](./multimodelserving/sklearn/README.md)  |


### Deploy InferenceService with Outlier/Drift Detector
In order to trust and reliably act on model predictions, it is crucial to monitor the distribution of the incoming
requests via various different type of detectors. KServe integrates [Alibi Detect](https://github.com/SeldonIO/alibi-detect) with the following components:
- Drift detector checks when the distribution of incoming requests is diverging from a reference distribution such as that of the training data 
- Outlier detector flags single instances which do not follow the training distribution.

| Features  | Examples |
| ------------- | ------------- |
| Deploy Alibi Outlier Detection| [Cifar outlier detector](./outlier-detection/alibi-detect/cifar10) |
| Deploy Alibi Drift Detection| [Cifar drift detector](./drift-detection/alibi-detect/cifar10) |

### Deploy InferenceService with Cloud/PVC storage
| Feature  | Examples |
| ------------- | ------------- |
| Deploy Model on S3| [Mnist model on S3](./storage/s3) |
| Deploy Model on PVC| [Models on PVC](./storage/pvc)  |
| Deploy Model on Azure| [Models on Azure](./storage/azure) |
| Deploy Model with HTTP/HTTPS| [Models with HTTP/HTTPS URL](./storage/uri) |

### Autoscaling
KServe's main serverless capability is to allow you to run inference workload without worrying about scaling your service manually once it is deployed. KServe leverages Knative's [autoscaler](https://knative.dev/docs/serving/configuring-autoscaling/),
the autoscaler works on GPU as well since the Autoscaler is based on request volume instead of GPU/CPU metrics which can be hard to reason about. 
 
[Autoscale inference workload on CPU/GPU](./autoscaling)

[InferenceService on GPU nodes](./accelerators)

### Canary Rollout
Canary deployment enables rollout releases by splitting traffic between different versions to ensure safe rollout.

[v1beta1 canary rollout](./v1beta1/rollout)

### Kubeflow Pipeline Integration
[InferenceService with Kubeflow Pipeline](./pipelines)

### Request Batching(Alpha)
Batching individual inference requests can be important as most of ML/DL frameworks are optimized for batch requests.
In cases where the services receive heavy load of requests, its advantageous to batch the requests. This allows for maximally
utilizing the CPU/GPU compute resource, but user needs to carefully perform enough tests to find optimal batch size and analyze 
the traffic patterns before enabling the batch inference. KServe injects a batcher sidecar so it can work with any model server
deployed on KServe, you can read more from this [example](./batcher).

### Request/Response Logger
KServe supports logging your inference request/response by injecting a sidecar alongside with your model server.

| Feature  | Examples |
| ------------- | ------------- |
| Deploy Logger with a Logger Service| [Message Dumper Service](./logger/basic)  |
| Deploy Async Logger| [Message Dumper Using Knative Eventing](./logger/knative-eventing)  |


### Deploy InferenceService behind an Authentication Proxy with Kubeflow
[InferenceService on Kubeflow with Istio-Dex](./istio-dex)

[InferenceService behind GCP Identity Aware Proxy (IAP) ](./gcp-iap)
