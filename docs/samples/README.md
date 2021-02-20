## KFServing Features and Examples

### Deploy InferenceService with Predictor
KFServing provides a simple Kubernetes CRD to allow deploying single or multiple trained models onto model servers such as [TFServing](https://www.tensorflow.org/tfx/guide/serving), 
[TorchServe](https://pytorch.org/serve/server.html), [ONNXRuntime](https://github.com/microsoft/onnxruntime), [Triton Inference Server](https://docs.nvidia.com/deeplearning/triton-inference-server/user-guide/docs).
In addition [KFServer](https://github.com/kubeflow/kfserving/tree/master/python/kfserving) is the Python model server implemented in KFServing itself with prediction v1 protocol,
[MLServer](https://github.com/SeldonIO/MLServer) implements the [prediction v2 protocol](https://github.com/kubeflow/kfserving/tree/master/docs/predict-api/v2) with both REST and gRPC.
These model servers are able to provide out-of-the-box model serving, but you could also choose to build your own model server for more complex use case.
KFServing provides basic API primitives to allow you easily build custom model server, you can use other tools like [BentoML](https://docs.bentoml.org/en/latest) to build your custom model serve image.

After models are deployed onto model servers with KFServing, you get all the following serverless features provided by KFServing.
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
| [TFServing](https://www.tensorflow.org/tfx/guide/serving) | [TensorFlow SavedModel](https://www.tensorflow.org/guide/saved_model) | v1 | :heavy_check_mark: | :heavy_check_mark: | [TFServing Versions](https://github.com/tensorflow/serving/releases) | [TensorFlow Examples](./v1alpha2/tensorflow)  |
| [TorchServe](https://pytorch.org/serve/server.html) | [Eager Model/TorchScript](https://pytorch.org/docs/master/generated/torch.save.html) | v1 | :heavy_check_mark: | :heavy_check_mark: | 0.3.0 | [TorchServe Examples](./v1beta1/torchserve)  |
| [TorchServe Native](https://pytorch.org/serve/server.html) | [Eager Model/TorchScript](https://pytorch.org/docs/master/generated/torch.save.html) | native | :heavy_check_mark: | :heavy_check_mark: | 0.3.0 | [TorchServe Examples](./v1beta1/custom/torchserve)  |
| [ONNXRuntime](https://github.com/microsoft/onnxruntime)  | [Exported ONNX Model](https://github.com/onnx/tutorials#converting-to-onnx-format) | v1 | :heavy_check_mark: | :heavy_check_mark: | [Compatibility](https://github.com/microsoft/onnxruntime#compatibility) |[ONNX Style Model](./v1alpha2/onnx)  |
| [SKLearn MLServer](https://github.com/SeldonIO/MLServer) | [Pickled Model](https://scikit-learn.org/stable/modules/model_persistence.html) | v2 | :heavy_check_mark: | :heavy_check_mark: | 0.23.1 | [SKLearn Iris V2](./v1beta1/v2/sklearn)  |
| [XGBoost MLServer](https://github.com/SeldonIO/MLServer) | [Saved Model](https://xgboost.readthedocs.io/en/latest/tutorials/saving_model.html) | v2 | :heavy_check_mark: | :heavy_check_mark: | 1.1.1 | [XGBoost Iris V2](./v1beta1/xgboost)  |
| [SKLearn KFServer](https://github.com/kubeflow/kfserving/tree/master/python/sklearnserver) | [Pickled Model](https://scikit-learn.org/stable/modules/model_persistence.html) | v1 | :heavy_check_mark: | -- | 0.20.3 | [SKLearn Iris](./v1beta1/v1/sklearn)  |
| [XGBoost KFServer](https://github.com/kubeflow/kfserving/tree/master/python/xgbserver) | [Saved Model](https://xgboost.readthedocs.io/en/latest/tutorials/saving_model.html) | v1 | :heavy_check_mark: | -- | 0.82 | [XGBoost Iris](./v1alpha2/xgboost)  |
| [PyTorch KFServer](https://github.com/kubeflow/kfserving/tree/master/python/pytorchserver) | [Eager Model](https://pytorch.org/docs/master/generated/torch.save.html) | v1 | :heavy_check_mark: | -- | 1.3.1 |  [PyTorch Cifar10](./v1alpha2/pytorch)  |
| [PMML KFServer](https://github.com/kubeflow/kfserving/tree/master/python/pmmlserver) | [PMML](http://dmg.org/pmml/v4-4-1/GeneralStructure.html) | v1 | :heavy_check_mark: | -- | [PMML4.4.1](https://github.com/autodeployai/pypmml) | [SKLearn PMML](./v1beta1/pmml)  |
| [LightGBM KFServer](https://github.com/kubeflow/kfserving/tree/master/python/lightgbm) | [Saved LightGBM Model](https://lightgbm.readthedocs.io/en/latest/pythonapi/lightgbm.Booster.html#lightgbm.Booster.save_model) | v1 | :heavy_check_mark: | -- | 2.3.1 | [LightGBM Iris](./v1beta1/lightgbm)  |

| Custom Predictor  | Examples |
| ------------- |  ------------- |
| Deploy model on custom KFServer | [Custom KFServer](./v1alpha2/custom/kfserving-custom-model)|
| Deploy model on BentoML | [SKLearn Iris with BentoML](./bentoml)|
| Deploy model on custom HTTP Server  | [Prebuilt Model Server](./v1alpha2/custom/prebuilt-image)|
| Deploy model on custom gRPC Server  | [Prebuilt gRPC Server](./v1alpha2/custom/grpc-server)|

In addition to deploy InferenceService with HTTP/gRPC endpoint, you can also deploy InferenceService with [Knative Event Sources](https://knative.dev/docs/eventing/sources/index.html) such as Kafka
, you can find an example [here](./kafka) which shows how to build an async inference pipeline. 

### Deploy InferenceService with Transformer
KFServing transformer enables users to define a pre/post processing step before the prediction and explanation workflow.
KFServing transformer runs as a separate microservice and can work with any type of pre-packaged model server, it can also 
scale differently from the predictor if your transformer is CPU bound while predictor requires running on GPU. 

| Features  | Examples |
| ------------- | ------------- |
| Deploy Transformer with KFServer | [Image Transformer with PyTorch KFServer](./v1alpha2/transformer/image_transformer)  |
| Deploy Transformer with Triton Server | [BERT Model with tokenizer](./v1beta1/triton/bert)  |
| Deploy Transformer with TorchServe| [Image classifier](./v1beta1/transformer/torchserve_image_transformer)  |

### Deploy InferenceService with Explainer
Model explainability answers the question: "Why did my model make this prediction" for a given instance. KFServing 
integrates with [Alibi Explainer](https://github.com/SeldonIO/alibi) which implements a black-box algorithm by generating a lot of similar looking intances 
for a given instance and send out to the model server to produce an explanation.

Additionally KFServing also integrates with The [AI Explainability 360 (AIX360)](https://ai-explainability-360.org/) toolkit, an LF AI Foundation incubation project, which is an open-source library that supports the interpretability and explainability of datasets and machine learning models. The AI Explainability 360 Python package includes a comprehensive set of algorithms that cover different dimensions of explanations along with proxy explainability metrics. In addition to native algorithms, AIX360 also provides algorithms from LIME and Shap.

| Features  | Examples |
| ------------- | ------------- |
| Deploy Alibi Image Explainer| [Imagenet Explainer](./explanation/alibi/imagenet)  |
| Deploy Alibi Income Explainer| [Income Explainer](./explanation/alibi/income)  |
| Deploy Alibi Text Explainer| [Alibi Text Explainer](./explanation/alibi/moviesentiment) |
| Deploy AIX360 Image Explainer| [AIX360 Image Explainer](./explanation/aix/mnist/README.md) |


### Deploy InferenceService with Outlier/Drift Detector
In order to trust and reliably act on model predictions, it is crucial to monitor the distribution of the incoming
requests via various different type of detectors. KFServing integrates [Alibi Detect](https://github.com/SeldonIO/alibi-detect) with the following components:
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
KFServing's main serverless capability is to allow you to run inference workload without worrying about scaling your service manually once it is deployed. KFServing leverages Knative's [autoscaler](https://knative.dev/docs/serving/configuring-autoscaling/),
the autoscaler works on GPU as well since the Autoscaler is based on request volume instead of GPU/CPU metrics which can be hard
 to reason about. 
 
[Autoscale inference workload on CPU/GPU](./autoscaling)

[InferenceService on GPU nodes](./accelerators)

### Canary Rollout
Canary deployment enables rollout releases by splitting traffic between different versions to ensure safe rollout.

[v1alpha2 canary rollout](./v1alpha2/rollouts)

[v1beta1 canary rollout](./v1beta1/rollout)

### Kubeflow Pipeline Integration
[InferenceService with Kubeflow Pipeline](./pipelines)

### Request Batching(Alpha)
Batching individual inference requests can be important as most of ML/DL frameworks are optimized for batch requests.
In cases where the services receive heavy load of requests, its advantageous to batch the requests. This allows for maximally
utilizing the CPU/GPU compute resource, but user needs to carefully perform enough tests to find optimal batch size and analyze 
the traffic patterns before enabling the batch inference. KFServing injects a batcher sidecar so it can work with any model server
deployed on KFServing, you can read more from this [example](./batcher).

### Request/Response Logger
KFServing supports logging your inference request/response by injecting a sidecar alongside with your model server.

| Feature  | Examples |
| ------------- | ------------- |
| Deploy Logger with a Logger Service| [Message Dumper Service](./logger/basic)  |
| Deploy Async Logger| [Message Dumper Using Knative Eventing](./logger/knative-eventing)  |


### Deploy InferenceService behind an Authentication Proxy with Kubeflow
[InferenceService on Kubeflow with Istio-Dex](./istio-dex)

[InferenceService behind GCP Identity Aware Proxy (IAP) ](./gcp-iap)
