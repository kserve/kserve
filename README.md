# KFServing
KFServing provides a Kubernetes [Custom Resource Definition](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/) for serving ML Models on arbitrary frameworks. It aims to solve 80% of model serving use cases by providing performant, high abstraction interfaces for common ML frameworks like Tensorflow, XGBoost, ScikitLearn, PyTorch, and custom containers. KFServing brings cutting edge serving features like Scale to Zero and Canary Rollouts to your ML deployments.

A KFService encapsulates the complexity of autoscaling, networking, health checking, server configuration, and more, to provide customers with a simple and seamless experience when deploying models.

In the future, we hope to support more advanced use cases such as skew detection, explainability, and performance profiling across infrastructure configurations.

This project is an evolution of the [original proposal in the Kubeflow repo](https://github.com/kubeflow/kubeflow/issues/2306). To know more about KFServing, please [read the docs](/docs)