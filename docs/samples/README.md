## KFServing Features and Examples

### Deploy InferenceService with Predictor
KFServing supports deploying models with pre-packaged model servers such as [TFServing](https://www.tensorflow.org/tfx/guide/serving), 
[ONNXRuntime](https://github.com/microsoft/onnxruntime), [Triton Inference Server](https://docs.nvidia.com/deeplearning/triton-inference-server/user-guide/docs),
[KFServer](https://github.com/kubeflow/kfserving/tree/master/python/kfserving)
which can load and serve a model. The advantage of doing this is that model deployment can be reused, deploying a new model with the same framework does not require writing code.
These model servers are also exposing a standardised API for both REST and gRPC. You could also choose to build your own model server for more complex use case,
KFServing provides basic API primitives to allow you easily build custom model server, you can use other tools like [BentoML](https://docs.bentoml.org/en/latest) to build your custom model serve image.
After model servers are deployed on KFServing, you get all the following serverless features provided by KFServing
- Scale to zero
- Request based Autoscaling on CPU/GPU
- Revision Management
- Optimized Container
- Batching and Logger
- Traffic management
- Security with AuthN/AuthZ
- Distributed Tracing
- Out-of-the-box metrics
- Ingress/Egress control

| Features  | Exported model| Examples |
| ------------- | ------------- | ------------- |
| Deploy SKLearn Model on KFServer | pickled model(model.pkl, model.joblib) | [SKLearn Iris](./sklearn)  |
| Deploy XGBoost Model on KFServer | pickled model(model.bst) | [XGBoost Iris](./xgboost)  |
| Deploy Pytorch Model on KFServer  | torch.save model(model.pt) | [PyTorch Cifar10](./pytorch)  |
| Deploy Tensorflow Model on TFServing  | Tensorflow SavedModel | [Tensorflow Flowers](./tensorflow)  |
| Deploy ONNX Model on ONNXRuntime  | exported onnx model(model.onnx) |[ONNX Style Model](./onnx)  |
| Deploy Model on Triton Server  | tensorflow/torch/onnx model| [Simple String](./triton/simple_string) |
| Deploy model on custom KFServer | | [Custom KFServer](./custom/kfserving-custom-model)|
| Deploy model on BentoML | | [SKLearn Iris with BentoML](./bentoml)|
| Deploy model on custom HTTP server | | [Prebuilt model server](./custom/prebuilt-image)|
| Deploy model with Kafka event source | | [Mnist model with Kafka Event Source](./kafka)

### Deploy InferenceService with Transformer
KFServing transformer enables users to define a pre/post processing step before the prediction and explanation workflow.
KFServing transformer runs as a separate microservice and can work with any type of pre-packaged model server, it can also 
scale differently from the predictor if your transformer is CPU bound while predictor requires running on GPU. 

| Features  | Examples |
| ------------- | ------------- |
| Deploy Transformer with KFServer | [Image Transformer with PyTorch KFServer](./transformer/image_transformer)  |
| Deploy Transformer with Triton Server | [BERT Model with transformer](./triton/bert)  |

### Deploy InferenceService with Explainer and Outlier/Drift Detector
Model explainability algorithm answers the question: "Why did my model make this prediction" for a given instance. 
In order to trust and reliably act on model predictions, it is crucial to monitor the distribution of the incoming
requests via various different type of detectors. Drift detector checks when the distribution of incoming requests 
is diverging from a reference distribution such as that of the training data.

| Features  | Examples |
| ------------- | ------------- |
| Deploy Alibi Image Explainer| [Imagenet Explainer](./explanation/alibi/imagenet)  |
| Deploy Alibi Income Explainer| [Income Explainer](./explanation/alibi/income)  |
| Deploy Alibi Text Explainer| [Alibi Text Explainer](./explanation/alibi/moviesentiment) |
| Deploy Alibi Detect| [Cifar outlier detector](./outlier-detection/alibi-detect/cifar10) |
| Deploy Alibi Drift detection| [Cifar drift detector](./drift-detection/alibi-detect/cifar10) |

### Deploy InferenceService with Cloud/PVC storage
| Feature  | Examples |
| ------------- | ------------- |
| Deploy Model on S3| [Mnist model on S3](./s3) |
| Deploy Model on PVC| [Models on PVC](./pvc)  |
| Deploy Model on Azure| [Models on Azure](./azure) |

### Autoscaling
KFServing's main serverless capability is to allow you run inference workload without worrying about the service
scaling once it is deployed. KFServing leverages Knative's [autoscaler](https://knative.dev/docs/serving/configuring-autoscaling/). 
The autoscaling works pretty well on GPU since the autoscaler is based on request volume instead of GPU/CPU metrics which can be hard
 to reason about. 
 
[Autoscale inference workload on CPU/GPU](./autoscaling)

[InferenceService on GPU nodes](./accelerators)

### Canary Rollout
Canary deployment enables rollout releases by splitting traffic between different versions.
[Canary Rollout](./rollouts)

### Kubeflow Pipeline Integration
[InferenceService with Kubeflow Pipeline](./pipelines)

### Request/Response Logger
[InferenceService with Request/Response Logger](./logger/basic)

### Deploy InferenceService behind an Authentication Proxy with Kubeflow
[InferenceService on Kubeflow with Istio-Dex](./istio-dex)

[InferenceService behind GCP Identity Aware Proxy (IAP) ](./gcp-iap)
