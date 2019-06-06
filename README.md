# KFServing
KFServing provides a Kubernetes [Custom Resource Definition](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/) for serving ML Models on arbitrary frameworks. It aims to solve 80% of model serving use cases by providing performant, high abstraction interfaces for common ML frameworks like Tensorflow, XGBoost, ScikitLearn, PyTorch, and custom containers. KFServing brings cutting edge serving features like GPU Autoscaling, Scale to Zero, and Canary Rollouts to your ML deployments.

A KFService encapsulates the complexity of autoscaling, networking, health checking, server configuration, and more, to provide customers with a simple and seamless experience when deploying models.

This project is an evolution of the [original proposal in the Kubeflow repo](https://github.com/kubeflow/kubeflow/issues/2306). 

### Learn More
* [Read the Docs](/docs)
* [KFServing 101](https://www.youtube.com/watch?v=hGIvlFADMhU)

### Contribute
* [Developer Guide](/docs/DEVELOPER_GUIDE.md).

![KFServing](./docs/diagrams/kfserving.png)