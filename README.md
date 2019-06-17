# KFServing
KFServing provides a Kubernetes [Custom Resource Definition](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/) for serving ML Models on arbitrary frameworks. It aims to solve 80% of model serving use cases by providing performant, high abstraction interfaces for common ML frameworks like Tensorflow, XGBoost, ScikitLearn, PyTorch, and ONNX as well as inference servers like TensorRT and custom container based servers. 

KFServing encapsulates the complexity of autoscaling, networking, health checking, and server configuration to bring cutting edge serving features like GPU Autoscaling, Scale to Zero, and Canary Rollouts to your ML deployments. It enables a simple, pluggable, and complete story for Mission Critical ML including inference, explainability, outlier detection, and prediction logging.

### Learn More
* [Read the Docs](/docs)
* [Roadmap](/ROADMAP.md)
* [KFServing 101 Slides](https://drive.google.com/file/d/16oqz6dhY5BR0u74pi9mDThU97Np__AFb/view)
* [KFServing 101 Tech Talk](https://www.youtube.com/watch?v=hGIvlFADMhU)
* This project is an evolution of the [original proposal in the Kubeflow repo](https://github.com/kubeflow/kubeflow/issues/2306). 

### Contribute
* [Developer Guide](/docs/DEVELOPER_GUIDE.md).

![KFServing](./docs/diagrams/kfserving.png)