# KServe
[![go.dev reference](https://img.shields.io/badge/go.dev-reference-007d9c?logo=go&logoColor=white)](https://pkg.go.dev/github.com/kserve/kserve)
[![Coverage Status](https://coveralls.io/repos/github/kserve/kserve/badge.svg?branch=master)](https://coveralls.io/github/kserve/kserve?branch=master)
[![Go Report Card](https://goreportcard.com/badge/github.com/kserve/kserve)](https://goreportcard.com/report/github.com/kserve/kserve)
[![Releases](https://img.shields.io/github/release-pre/kserve/kserve.svg?sort=semver)](https://github.com/kserve/kserve/releases)
[![LICENSE](https://img.shields.io/github/license/kserve/kserve.svg)](https://github.com/kserve/kserve/blob/master/LICENSE)
[![Slack Status](https://img.shields.io/badge/slack-join_chat-white.svg?logo=slack&style=social)](https://kubeflow.slack.com/join/shared_invite/zt-cpr020z4-PfcAue_2nw67~iIDy7maAQ)

KServe provides a Kubernetes [Custom Resource Definition](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/) for serving machine learning (ML) models on arbitrary frameworks. It aims to solve production model serving use cases by providing performant, high abstraction interfaces for common ML frameworks like Tensorflow, XGBoost, ScikitLearn, PyTorch, and ONNX.

It encapsulates the complexity of autoscaling, networking, health checking, and server configuration to bring cutting edge serving features like GPU Autoscaling, Scale to Zero, and Canary Rollouts to your ML deployments. It enables a simple, pluggable, and complete story for Production ML Serving including prediction, pre-processing, post-processing and explainability. KServe is being [used across various organizations.](https://kserve.github.io/website/community/adopters/)

For more details, visit [KServe website](https://kserve.github.io/website/)

![KServe](/docs/diagrams/kserve.png)

_Since 0.7 [KFServing is rebranded to KServe](https://blog.kubeflow.org/release/official/2021/09/27/kfserving-transition.html), we still support previous KFServing [0.5.x](https://github.com/kserve/kserve/tree/release-0.5) and 
[0.6.x](https://github.com/kserve/kserve/tree/release-0.6) releases, please refer to corresponding release branch for docs_.

### Learn More
To learn more about KServe, how to deploy it as part of Kubeflow, how to use various supported features, and how to participate in the KServe community, 
please follow the [KServe website documentation](https://kserve.github.io/website). 
Additionally, we have compiled a list of [presentations and demoes](/docs/PRESENTATIONS.md) to dive through various details.

### Installation

#### Standalone Installation
KServe by default installs Knative for serverless deployment, please follow [Serverless installation guide](https://kserve.github.io/website/admin/serverless) to
install KServe. If you are looking to install KServe without Knative(this feature is still alpha), please follow [Raw Kubernetes Deployment installation guide](https://kserve.github.io/website/admin/kubernetes_deployment).

#### Quick Install
Please follow [quick install](https://kserve.github.io/website/get_started) to install KServe on your local machine.

#### Create test inference service

Please follow [getting started](https://kserve.github.io/website/get_started/first_isvc) to create your first `InferenceService`.

### Roadmap
[Roadmap](./ROADMAP.md)

### API Reference
[InferenceService v1beta1 API Docs](https://kserve.github.io/website/reference/api)

### Developer Guide
[Developer Guide](https://kserve.github.io/website/developer/developer/).

### Contributor Guide
[Contributor Guide](./CONTRIBUTING.md)

### Adopters
[Adopters](https://kserve.github.io/website/community/adopters/)
