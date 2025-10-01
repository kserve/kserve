# KServe
[![go.dev reference](https://img.shields.io/badge/go.dev-reference-007d9c?logo=go&logoColor=white)](https://pkg.go.dev/github.com/kserve/kserve)
[![Coverage Status](https://img.shields.io/endpoint?url=https://gist.githubusercontent.com/andyi2it/5174bd748ac63a6e4803afea902e9810/raw/coverage.json)](https://github.com/kserve/kserve/actions/workflows/go.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/kserve/kserve)](https://goreportcard.com/report/github.com/kserve/kserve)
[![OpenSSF Best Practices](https://bestpractices.coreinfrastructure.org/projects/6643/badge)](https://bestpractices.coreinfrastructure.org/projects/6643)
[![Releases](https://img.shields.io/github/release-pre/kserve/kserve.svg?sort=semver)](https://github.com/kserve/kserve/releases)
[![LICENSE](https://img.shields.io/github/license/kserve/kserve.svg)](https://github.com/kserve/kserve/blob/master/LICENSE)
[![Slack Status](https://img.shields.io/badge/slack-join_chat-white.svg?logo=slack&style=social)](https://github.com/kserve/community/blob/main/README.md#questions-and-issues)
[![Gurubase](https://img.shields.io/badge/Gurubase-Ask%20KServe%20Guru-006BFF)](https://gurubase.io/g/kserve)

KServe is a standardized distributed generative and predictive AI inference platform for scalable, multi-framework deployment on Kubernetes.

KServe is being [used across various organizations](https://kserve.github.io/website/docs/community/adopters) and is a [Cloud Native Computing Foundation (CNCF)](https://www.cncf.io/) incubating project.

For more details, visit the [KServe website](https://kserve.github.io/website/).

![KServe](/docs/diagrams/kserve_new.png)

### Why KServe?

Single platform that unifies Generative and Predictive AI inference on Kubernetes. Simple enough for quick deployments, yet powerful enough to handle enterprise-scale AI workloads with advanced features.

### Features

**Generative AI**
  * 🧠 **LLM-Optimized**: OpenAI-compatible inference protocol for seamless integration with large language models
  * 🚅 **GPU Acceleration**: High-performance serving with GPU support and optimized memory management for large models
  * 💾 **Model Caching**: Intelligent model caching to reduce loading times and improve response latency for frequently used models
  * 🗂️ **KV Cache Offloading**: Advanced memory management with KV cache offloading to CPU/disk for handling longer sequences efficiently
  * 📈 **Autoscaling**: Request-based autoscaling capabilities optimized for generative workload patterns
  * 🔧 **Hugging Face Ready**: Native support for Hugging Face models with streamlined deployment workflows

**Predictive AI**
  * 🧮 **Multi-Framework**: Support for TensorFlow, PyTorch, scikit-learn, XGBoost, ONNX, and more
  * 🔀 **Intelligent Routing**: Seamless request routing between predictor, transformer, and explainer components with automatic traffic management
  * 🔄 **Advanced Deployments**: Canary rollouts, inference pipelines, and ensembles with InferenceGraph
  * ⚡ **Autoscaling**: Request-based autoscaling with scale-to-zero for predictive workloads
  * 🔍 **Model Explainability**: Built-in support for model explanations and feature attribution to understand prediction reasoning
  * 📊 **Advanced Monitoring**: Enables payload logging, outlier detection, adversarial detection, and drift detection
  * 💰 **Cost Efficient**: Scale-to-zero on expensive resources when not in use, reducing infrastructure costs

### Learn More
To learn more about KServe, how to use various supported features, and how to participate in the KServe community, 
please follow the [KServe website documentation](https://kserve.github.io/website). 
Additionally, we have compiled a list of [presentations and demos](https://kserve.github.io/website/docs/community/presentations) to dive through various details.

### :hammer_and_wrench: Installation

#### Standalone Installation
- **[Standard Kubernetes Installation](https://kserve.github.io/website/docs/admin-guide/overview#raw-kubernetes-deployment)**: Compared to Serverless Installation, this is a more **lightweight** installation. However, this option does not support canary deployment and request based autoscaling with scale-to-zero.
- **[Knative Installation](https://kserve.github.io/website/docs/admin-guide/overview#serverless-deployment)**: KServe by default installs Knative for **serverless deployment** for InferenceService.
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

### Star History

[![Star History Chart](https://api.star-history.com/svg?repos=kserve/kserve&type=Date)](https://www.star-history.com/#kserve/kserve&Date)
