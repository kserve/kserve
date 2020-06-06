# KF Serving Roadmap
## 2019 - 2020

### v0.4 Performance (ETA: Jan 31, 2019)
Objective: "Prevent performance regressions across a known set of representative models."
* Automated Performance Tests 
    * Define a set of Models to test covering a wide array of usecases and frameworks.
    * Publish performance results over time to enable regression tracking.

Objective: "Enable users to deploy latency sensitive models with KFServing."
* High Performance Dataplane
    * Enable support for GRPC or similar.
    * Continue to support existing HTTP Dataplane.

Objective: "Reduce Total Cost of Ownership when deploying multiple underutilized models."
* GPU Sharing 
    * Reduce TCO by enabling models of the same framework and version to be co-hosted in a single model server.

### v0.3 Stability (ETA: Dec 15, 2019)
Objective: "Improve practices around dependency management." 
* Migrate to Kubebuilder 2.0.
    * Use Go Modules.
    * Stop Vendoring dependencies.
    * Avoid the extremely heavy dependency on Tensorflow.
* Migrate to Kubernetes 1.15.
    * Enable LabelSelectors for the Pod Mutation Webhook.

Objective: "Prevent feature regressions with greater end-to-end test coverage against a live cluster."
* Automated End-to-End Tests
    * Execute against a Kubeflow maintained GKE Cluster.
    * Execute against a Kubeflow maintained AKS Cluster.
    * Achieve >80% Test Coverage of Supported Features.

Objective: "Improve build and release processes to improve the developer experience and avoid regressions."
* Improve build reliability
    * Implement build retries.
    * Reduce PyTorch build time.
* Automated Image Injection for Model Servers.
    * Implement new developer commands to deploy kfserving with local images.
* Improve versioning of XGBoost, SKLearn, and PyTorch
    * Replace KFServing version with the corresponding framework version.

# Future 
## Unscheduled Work
* Flexible Inference Graphs [MLGraph CRD](https://github.com/SeldonIO/mlgraph).
    * Model Experimentation.
    * Ensembling.
    * Multi Arm Bandit.
* Payload Logging
    * Finalize the design and implementation for [Payload Logging](https://docs.google.com/document/d/1MBl5frM9l_wyQkYEaDeHOP6Mrsuz9YOob7276AAN9_c/edit?usp=sharing)
* Bias, Skew, and Outlier Detection.
    * Online support in graph.
    * Offline support with Payload Logging.
* Meta-Protocol Definition.
    * Unify disparate protocols across frameworks.
* Adaptive batching support
    * Queue and batch requests to increase throughput.

# Historical
### v0.2 Integrate with the ML Ecosystem (Oct 31, 2019)
Objective: "Continue to simplify the user experience by deeply integrating with the Kubeflow Ecosystem."
* Kubeflow Integration
    * Prepare KFServing to release v0.2 and v0.3 alongside Kubeflow v0.7.
    * Integrate with `kfctl generate` and `kfctl apply`.
    * Deploy as a [Kubernetes Application](https://github.com/kubernetes-sigs/application).
    * Integrate with Kubeflow Pipelines to enable model deployment from a Pipeline.
    * Integrate with Fairing to enable model deployment from a Notebook.
    * Achieve 20% End-to-End Test Coverage of Supported Features. (See v0.3 for 80%).
    * Support PVCs to enable integration with on-prem Kubeflow installations.
    * Document Installation for various cloud providers (GCP, IBM Cloud, Azure, AWS).

Objective: "Empower users to deeply understand their predictions and validate KFServing's static graph architecture."
* Explainability
    * Deploy a predictor and explainer, powered by Alibi.
    * Deploy a predictor and explainer, powered by user specified explainer container.

Objective: "Increase coverage of ML frameworks to support previously unsupported customer workloads."
* Frameworks
    * Deploy a ONNX model
    * Explore supporting other model serialization mechanisms for certain frameworks (e.g. saving PyTorch models with dill)



## Q2 2019
### v0.1: InferenceService Minimum Viable Product (June 30, 2019)
Objective: "Simplify the user experience and provide a low barrier to entry by minimizing the amount of YAML necessary to deploy a trained model."
* High Level Interfaces
    * Deploy a Tensorflow model without specifying a Tensorflow Serving Technology.
    * Deploy a XGBoost model without specifying a XGBoost Serving Technology.
    * Deploy a ScikitLearn model without specifying a ScikitLearn Serving Technology.
    * Deploy a Pytorch model without specifying a Pytorch Serving Technology.
    * Deploy a Custom Containerized model by specifying your docker image and args.

Objective: "Empower users to safely deploy production models by enabling a variety of deployment strategies." 
* Model Rollout
    * Rollout a model using a blue-green strategy.
    * Rollout a model using a pinned strategy.
    * Rollout a model using a canary strategy.

Objective: "Reduce the total cost of ownership for models by minimizing the delta between provisioned resources and request load."
* Autoscaling 
    * Scale a model to zero.
    * Scale a model from zero without dropping traffic.
    * Scale a model that is GPU bound.
    * Scale a model that is CPU bound.