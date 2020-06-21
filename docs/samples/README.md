## KFServing Features and Examples

### Deploy InferenceService with Predictor
| Feature  | Exported model| Examples |
| ------------- | ------------- | ------------- |
| Deploy SKLearn Model on KFServer | pickled model(model.pkl, model.joblib) | [SKLearn Iris](./sklearn)  |
| Deploy XGBoost Model on KFServer | pickled model(model.bst) | [XGBoost Iris](./xgboost)  |
| Deploy Tensorflow Model on TFServing  | Tensorflow SavedModel | [Tensorflow Flowers](./tensorflow)  |
| Deploy Pytorch Model on KFServer  | torch.save model(model.pt) | [PyTorch Cifar10](./pytorch)  |
| Deploy ONNX Model on ONNXRuntime  | exported onnx model(model.onnx) |[Style ONNX model](./onnx)  |
| Deploy Model on Triton Server  | tensorflow/torch/onnx model| [Simple String Model](./triton/simple_string) |
| Deploy model on custom KFServer | | [Custom KFServer](./custom/kfserving-custom-model)|
| Deploy model on BentoML | | [SKlearn Iris with BentoML](./bentoml)|
| Deploy model on custom HTTP server | | [Prebuilt model server](./custom/prebuilt-image)|
| Deploy model with Kafka event source | | [Mnist model with Kafka Event Source](./kafka)

### Deploy InferenceService with Transformer
| Feature  | Examples |
| ------------- | ------------- |
| Deploy Transformer with KFServer | [Image Transformer with PyTorch KFServer](./transformer/image_transformer)  |
| Deploy Transformer with Triton Server | [BERT Model with transformer](./triton/bert)  |

### Deploy InferenceService with Explainer and Outlier/Drift Detector
| Feature  | Examples |
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

### Autoscaling, Canary Rollout
[Autoscale inference workload on CPU/GPU](./autoscaling)

[InferenceService on GPU nodes](./accelerators)

[Canary Rollout](./rollouts)

### Kubeflow Pipeline Integration
[InferenceService with Kubeflow Pipeline](./pipelines)

### Request/Response Logger
[InferenceService with Request/Response Logger](./logger/basic)

### Deploy InferenceService behind an Authentication Proxy with Kubeflow
[InferenceService on Kubeflow with Istio-Dex](./istio-dex)

[InferenceService behind GCP Identity Aware Proxy (IAP) ](./gcp-iap)
