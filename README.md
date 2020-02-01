# KFServing
KFServing provides a Kubernetes [Custom Resource Definition](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/) for serving machine learning (ML) models on arbitrary frameworks. It aims to solve production model serving use cases by providing performant, high abstraction interfaces for common ML frameworks like Tensorflow, XGBoost, ScikitLearn, PyTorch, and ONNX.

It encapsulates the complexity of autoscaling, networking, health checking, and server configuration to bring cutting edge serving features like GPU Autoscaling, Scale to Zero, and Canary Rollouts to your ML deployments. It enables a simple, pluggable, and complete story for Production ML Serving including prediction, pre-processing, post-processing and explainability.

![KFServing](/docs/diagrams/kfserving.png)

### Learn More
To learn more about KFServing, how to deploy it as part of Kubeflow, how to use various supported features, and how to participate in the KFServing community, please follow the [KFServing docs on the Kubeflow Website](https://www.kubeflow.org/docs/components/serving/kfserving/).

### Prerequisites
Knative Serving and Istio should be available on Kubernetes Cluster, Knative depends on an Ingress/Gateway to route requests to Knative services.
- [Istio](https://knative.dev/docs/install/installing-istio): v1.1.7+

If you want to get up running Knative quickly or you do not need service mesh, we recommend installing Istio without service mesh(sidecar injection).
- [Knative Serving](https://knative.dev/docs/install/knative-with-any-k8s): v0.9.x+

Currently only `Knative Serving` is required, `Knative Eventing` is not required unless your events are from a particular system such as Kafka.
Note that `cluster-local-gateway` is required to serve cluster-internal traffic for transformer and explainer use cases.

- [Cert Manager](https://cert-manager.io/docs/installation/kubernetes): v1.12.0+

Cert manager is needed to provision KFServing webhook certs for production grade installation, alternatively you can run our self signed certs generation script.

You may find this [installation instruction](https://github.com/kubeflow/kfserving/blob/master/docs/DEVELOPER_GUIDE.md#install-knative-on-a-kubernetes-cluster) useful.

### Installation using kubectl
```
TAG=0.2.2
kubectl apply -f ./install/$TAG/kfserving.yaml
```
By default, you can create InferenceService instances in any namespace which has no label with `control-plane` as key.
You can also configure KFServing to make InferenceService instances only work in the namespace which has label pair `serving.kubeflow.org/inferenceservice: enabled`. To enable this mode, you need to add `env` field as stated below to `manager` container of statefulset `kfserving-controller-manager`.

```
env:
- name: ENABLE_WEBHOOK_NAMESPACE_SELECTOR
  value: enabled
```
Please refer to our [troubleshooting section](docs/DEVELOPER_GUIDE.md#troubleshooting) for recommendations and tips.

### Examples
[Deploy SKLearn Model with out-of-the-box InferenceService](./docs/samples/sklearn)

[Deploy PyTorch Model with out-of-the-box InferenceService](./docs/samples/pytorch)

[Deploy Tensorflow Model with out-of-the-box InferenceService](./docs/samples/tensorflow)

[Deploy XGBoost Model with out-of-the-box InferenceService](./docs/samples/xgboost)

[Deploy ONNX Model with ONNX Runtime InferenceService](./docs/samples/onnx)

[Deploy Deep Learning Models with NVIDIA's TensorRT InferenceService](./docs/samples/tensorrt)

[Deploy KFServing InferenceService with Transformer and Predictor](./docs/samples/transformer/image_transformer)

[Deploy KFServing InferenceService with Models on S3](./docs/samples/s3)

[Deploy KFServing InferenceService with Models on PVC](./docs/samples/pvc)

[Deploy KFServing InferenceService with Models on Azure](./docs/samples/azure)

[Deploy KFServing InferenceService on GPU nodes](./docs/samples/accelerators)

[Deploy KFServing InferenceService with Canary Rollout](./docs/samples/rollouts)

[Deploy KFServing InferenceService with Kubeflow Pipeline](./docs/samples/pipelines)

[Deploy KFServing InferenceService  with Request/Response Logger](./docs/samples/logger)

[Deploy KFServing InferenceService with Kafka Event Source](./docs/samples/kafka)

[Deploy KFServing InferenceService with Alibi Image Explainer](./docs/samples/explanation/alibi/imagenet)

[Deploy KFServing InferenceService with Alibi Text Explainer](./docs/samples/explanation/alibi/moviesentiment)

[Deploy KFServing InferenceService with Alibi Tabular Explainer](./docs/samples/explanation/alibi/income)

[Autoscale KFServing InferenceService with your inference workload on CPU/GPU](./docs/samples/autoscaling)

### Use SDK
* Install the SDK
  ```
  pip install kfserving
  ```
* Get the KFServing SDK documents from [here](python/kfserving/README.md).

* Follow the [example here](docs/samples/client/kfserving_sdk_sample.ipynb) to use the KFServing SDK to create, rollout, promote, and delete an InferenceService instance.

### Contribute
* [Developer Guide](/docs/DEVELOPER_GUIDE.md).
