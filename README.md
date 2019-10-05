# KFServing
KFServing provides a Kubernetes [Custom Resource Definition](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/) for serving machine learning (ML) models on arbitrary frameworks. It aims to solve production model serving use cases by providing performant, high abstraction interfaces for common ML frameworks like Tensorflow, XGBoost, ScikitLearn, PyTorch, and ONNX.

KFServing encapsulates the complexity of autoscaling, networking, health checking, and server configuration to bring cutting edge serving features like GPU Autoscaling, Scale to Zero, and Canary Rollouts to your ML deployments. It enables a simple, pluggable, and complete story for production ML Inference Server by providing prediction, pre-processing, post-processing and explainability out of the box.

![KFServing](https://www.kubeflow.org/docs/components/serving/kfserving.png)

### Learn More
To learn more about KFServing, how to deploy it as part of Kubeflow, how to use various supported features, and how to participate in the KFServing community, please follow the [KFServing docs on the KubefloW Website](https://www.kubeflow.org/docs/components/serving/kfserving/).

### Prerequisites
KNative Serving and Istio should be available on Kubernetes Cluster.
- Istio Version: v1.1.7+ 
- Knative Version: v0.8.x

You may find this [installation instruction](https://github.com/kubeflow/kfserving/blob/master/docs/DEVELOPER_GUIDE.md#install-knative-on-a-kubernetes-cluster) useful.

### Installation using kubectl ###
```
TAG=v0.1.0
kubectl apply -f ./install/$TAG/kfserving.yaml
```

### Use ###
* Install the SDK
```
pip install kfserving
```
* Follow the [example here](docs/samples/client/kfserving_sdk_sample.ipynb) to use the KFServing SDK to create, patch, and delete a KFService instance.

### Contribute
* [Developer Guide](/docs/DEVELOPER_GUIDE.md).
