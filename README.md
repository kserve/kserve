# KFServing
KFServing provides a [Custom Resource Definition](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/) for serving ML Models on arbitrary frameworks. It aims to solve majority of model serving use cases by providing performant, high abstraction interfaces for common ML frameworks like Tensorflow, XGBoost, ScikitLearn, PyTorch, and custom containers. Using KFServing you should be able to deploy, scale up or down to zero and perform A/B tests on your models in a consistent and standardized way.

A KFService encapsulates the complexity of autoscaling, networking, health checking, server configuration, and more, to provide customers with a simple and seamless experience when deploying models.

In the future, we hope to support more advanced use cases such as skew detection, explainability, and performance profiling across infrastructure configurations.

This project is an evolution of the original proposal: https://github.com/kubeflow/kubeflow/issues/2306
