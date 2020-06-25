## KFServing Features and Examples

### Deploy InferenceService with Predictor
KFServing provides a simple Kubernetes CRD to allow deploying trained models onto model servers such as [TFServing](https://www.tensorflow.org/tfx/guide/serving), 
[ONNXRuntime](https://github.com/microsoft/onnxruntime), [Triton Inference Server](https://docs.nvidia.com/deeplearning/triton-inference-server/user-guide/docs),
[KFServer](https://github.com/kubeflow/kfserving/tree/master/python/kfserving). These model servers are also exposing a standardised API for both REST and gRPC. You could also choose to build your own model server for more complex use case,
KFServing provides basic API primitives to allow you easily build custom model server, you can use other tools like [BentoML](https://docs.bentoml.org/en/latest) to build your custom model serve image.
After models are deployed onto model servers with KFServing, you get all the following serverless features provided by KFServing
- Scale to and from Zero
- Request based Autoscaling on CPU/GPU
- Revision Management
- Optimized Container
- Batching and Logger
- Traffic management
- Security with AuthN/AuthZ
- Distributed Tracing
- Out-of-the-box metrics
- Ingress/Egress control

|   | Exported model| HTTP | gRPC | Examples |
| ------------- | ------------- | ------------- | ------------- | ------------- |
| Deploy SKLearn Model on KFServer | pickled model(model.pkl, model.joblib) | :heavy_check_mark: | V2 |[SKLearn Iris](./sklearn)  |
| Deploy XGBoost Model on KFServer | pickled model(model.bst) | :heavy_check_mark: | V2 |[XGBoost Iris](./xgboost)  |
| Deploy Pytorch Model on KFServer  | [torch.save model(model.pt)](https://pytorch.org/docs/master/generated/torch.save.html) | :heavy_check_mark: | V2 |  [PyTorch Cifar10](./pytorch)  |
| Deploy Tensorflow Model on TFServing  | [Tensorflow SavedModel](https://www.tensorflow.org/guide/saved_model) | :heavy_check_mark: | :heavy_check_mark: | [Tensorflow Flowers](./tensorflow)  |
| Deploy ONNX Model on ONNXRuntime  | [Exported onnx model(model.onnx)](https://github.com/onnx/tutorials#converting-to-onnx-format) | :heavy_check_mark: | :heavy_check_mark: |[ONNX Style Model](./onnx)  |
| Deploy Model on Triton Server | [Tensorflow,PyTorch,ONNX,TensorRT](https://docs.nvidia.com/deeplearning/triton-inference-server/user-guide/docs/model_repository.html)| :heavy_check_mark: | :heavy_check_mark: | [Simple String](./triton/simple_string) |
| Deploy model on custom KFServer | | :heavy_check_mark: | V2 | [Custom KFServer](./custom/kfserving-custom-model)|
| Deploy model on BentoML | | :heavy_check_mark: | - | [SKLearn Iris with BentoML](./bentoml)|
| Deploy model on custom HTTP Server | |:heavy_check_mark: | - | [Prebuilt model server](./custom/prebuilt-image)|
| Deploy model on custom gRPC Server | |  -  | :heavy_check_mark: | [Prebuilt gRPC server](./custom/grpc-server)|

In addition to deploy InferenceService with HTTP/gRPC endpoint, you can also deploy InferenceService with [Knative Event Sources](https://knative.dev/docs/eventing/sources/index.html) such as Kafka
, you can find an example [here](./kafka) which shows how to build an async inference pipeline. 

### Deploy InferenceService with Transformer
KFServing transformer enables users to define a pre/post processing step before the prediction and explanation workflow.
KFServing transformer runs as a separate microservice and can work with any type of pre-packaged model server, it can also 
scale differently from the predictor if your transformer is CPU bound while predictor requires running on GPU. 

| Features  | Examples |
| ------------- | ------------- |
| Deploy Transformer with KFServer | [Image Transformer with PyTorch KFServer](./transformer/image_transformer)  |
| Deploy Transformer with Triton Server | [BERT Model with tokenizer](./triton/bert)  |

### Deploy InferenceService with Explainer and Outlier/Drift Detector
Model explainability answers the question: "Why did my model make this prediction" for a given instance. KFServing 
integrates with [Alibi Explainer](https://github.com/SeldonIO/alibi) which implements a black-box algorithm by generating a lot of similar looking intances 
for a given instance and send out to the model server to produce an explanation.
Also in order to trust and reliably act on model predictions, it is crucial to monitor the distribution of the incoming
requests via various different type of detectors. [Alibi Detect](https://github.com/SeldonIO/alibi-detect) checks when the distribution of incoming requests 
is diverging from a reference distribution such as that of the training data.

| Features  | Examples |
| ------------- | ------------- |
| Deploy Alibi Image Explainer| [Imagenet Explainer](./explanation/alibi/imagenet)  |
| Deploy Alibi Income Explainer| [Income Explainer](./explanation/alibi/income)  |
| Deploy Alibi Text Explainer| [Alibi Text Explainer](./explanation/alibi/moviesentiment) |
| Deploy Alibi Outlier Detection| [Cifar outlier detector](./outlier-detection/alibi-detect/cifar10) |
| Deploy Alibi Drift Detection| [Cifar drift detector](./drift-detection/alibi-detect/cifar10) |

### Deploy InferenceService with Cloud/PVC storage
| Feature  | Examples |
| ------------- | ------------- |
| Deploy Model on S3| [Mnist model on S3](./s3) |
| Deploy Model on PVC| [Models on PVC](./pvc)  |
| Deploy Model on Azure| [Models on Azure](./azure) |

### Autoscaling
KFServing's main serverless capability is to allow you to run inference workload without worrying about scaling your service manually once it is deployed. KFServing leverages Knative's [autoscaler](https://knative.dev/docs/serving/configuring-autoscaling/),
the autoscaler works on GPU as well since the Autoscaler is based on request volume instead of GPU/CPU metrics which can be hard
 to reason about. 
 
[Autoscale inference workload on CPU/GPU](./autoscaling)

[InferenceService on GPU nodes](./accelerators)

### Canary Rollout
Canary deployment enables rollout releases by splitting traffic between different versions to ensure safe rollout.

[Canary Rollout](./rollouts)

### Kubeflow Pipeline Integration
[InferenceService with Kubeflow Pipeline](./pipelines)

### Request Batching and Request/Response Logger
KFServing supports batching incoming requests to increase throughput and logging your inference request/response by injecting a sidecar alongside with your model server.

| Features  | Examples |
| ------------- | ------------- |
| Request Batching| [Batcher](./batcher)  |
| Deploy Logger with a Logger Service| [Message Dumper Service](./logger/basic)  |
| Deploy Async Logger| [Message Dumper Using Knative Eventing](./logger/knative-eventing)  |


### Deploy InferenceService behind an Authentication Proxy with Kubeflow
[InferenceService on Kubeflow with Istio-Dex](./istio-dex)

[InferenceService behind GCP Identity Aware Proxy (IAP) ](./gcp-iap)
