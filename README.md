# KServe
[![go.dev reference](https://img.shields.io/badge/go.dev-reference-007d9c?logo=go&logoColor=white)](https://pkg.go.dev/github.com/kserve/kserve)
[![Coverage Status](https://img.shields.io/endpoint?url=https://gist.githubusercontent.com/andyi2it/5174bd748ac63a6e4803afea902e9810/raw/coverage.json)](https://github.com/kserve/kserve/actions/workflows/go.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/kserve/kserve)](https://goreportcard.com/report/github.com/kserve/kserve)
[![OpenSSF Best Practices](https://bestpractices.coreinfrastructure.org/projects/6643/badge)](https://bestpractices.coreinfrastructure.org/projects/6643)
[![Releases](https://img.shields.io/github/release-pre/kserve/kserve.svg?sort=semver)](https://github.com/kserve/kserve/releases)
[![LICENSE](https://img.shields.io/github/license/kserve/kserve.svg)](https://github.com/kserve/kserve/blob/master/LICENSE)
[![Slack Status](https://img.shields.io/badge/slack-join_chat-white.svg?logo=slack&style=social)](https://github.com/kserve/community/blob/main/README.md#questions-and-issues)
[![Gurubase](https://img.shields.io/badge/Gurubase-Ask%20KServe%20Guru-006BFF)](https://gurubase.io/g/kserve)

KServe provides a Kubernetes [Custom Resource Definition](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/) for serving predictive and generative machine learning (ML) models. It aims to solve production model serving use cases by providing high abstraction interfaces for Tensorflow, XGBoost, ScikitLearn, PyTorch, Huggingface Transformer/LLM models using standardized data plane protocols.

It encapsulates the complexity of autoscaling, networking, health checking, and server configuration to bring cutting edge serving features like GPU Autoscaling, Scale to Zero, and Canary Rollouts to your ML deployments. It enables a simple, pluggable, and complete story for Production ML Serving including prediction, pre-processing, post-processing and explainability. KServe is being [used across various organizations.](https://kserve.github.io/website/docs/community/adopters)

For more details, visit the [KServe website](https://kserve.github.io/website/).

![KServe](/docs/diagrams/kserve_new.png)

*[KFServing has been rebranded to KServe since v0.7](https://blog.kubeflow.org/release/official/2021/09/27/kfserving-transition.html).*

### Why KServe?
- KServe is a standard, cloud agnostic **Model Inference Platform** for serving predictive and generative AI models on Kubernetes, built for highly scalable use cases.
- Provides performant, **standardized inference protocol** across ML frameworks including OpenAI specification for generative models.
- Support modern **serverless inference workload** with **request based autoscaling including scale-to-zero** on **CPU and GPU**.
- Provides **high scalability, density packing and intelligent routing** using **ModelMesh**.
- **Simple and pluggable production serving** for **inference**, **pre/post processing**, **monitoring** and **explainability**.
- Advanced deployments for **canary rollout**, **pipeline**, **ensembles** with **InferenceGraph**.

### Learn More
To learn more about KServe, how to use various supported features, and how to participate in the KServe community, 
please follow the [KServe website documentation](https://kserve.github.io/website). 
Additionally, we have compiled a list of [presentations and demos](https://kserve.github.io/website/docs/community/presentations) to dive through various details.

### :hammer_and_wrench: Installation

#### Standalone Installation
- **[Serverless Installation](https://kserve.github.io/website/docs/admin-guide/overview#serverless-deployment)**: KServe by default installs Knative for **serverless deployment** for InferenceService.
- **[Raw Deployment Installation](https://kserve.github.io/website/docs/admin-guide/overview#raw-kubernetes-deployment)**: Compared to Serverless Installation, this is a more **lightweight** installation. However, this option does not support canary deployment and request based autoscaling with scale-to-zero.
- **[ModelMesh Installation](https://kserve.github.io/website/docs/admin-guide/overview#modelmesh-deployment)**: You can optionally install ModelMesh to enable **high-scale**, **high-density** and **frequently-changing model serving** use cases. 
- **[Quick Installation](https://kserve.github.io/website/docs/getting-started/quickstart-guide)**: Install KServe on your local machine.

#### Kubeflow Installation
KServe is an important addon component of Kubeflow, please learn more from the [Kubeflow KServe documentation](https://www.kubeflow.org/docs/external-add-ons/kserve/kserve). Check out the following guides for running [on AWS](https://awslabs.github.io/kubeflow-manifests/main/docs/component-guides/kserve) or [on OpenShift Container Platform](https://github.com/kserve/kserve/blob/master/docs/OPENSHIFT_GUIDE.md).

### :flight_departure: [Create your first InferenceService](https://kserve.github.io/website/docs/getting-started/genai-first-isvc)

### :bulb: [Roadmap](./ROADMAP.md)

### :blue_book: [InferenceService API Reference](https://kserve.github.io/website/docs/reference/crd-api)

### :toolbox: [Developer Guide](https://kserve.github.io/website/docs/developer-guide)

### :writing_hand: [Contributor Guide](https://kserve.github.io/website/docs/developer-guide/contribution)

### :handshake: [Adopters](https://kserve.github.io/website/docs/community/adopters)
