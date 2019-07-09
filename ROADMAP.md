# KF Serving 2019 Roadmap
## Q3 2019
### v0.2 Integrate with the ML Ecosystem (ETA: August 15, 2019)
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

### v0.3 Performance and Stability (ETA: September 1, 2019)
Objective: "Prevent feature regressions with 80% end-to-end test coverage against a live Cluster."
* Automated End-to-End Tests
    * Execute against a Kubeflow maintained GKE Cluster.
    * Execute against a Kubeflow maintained AKS Cluster.
    * Achieve >80% Test Coverage of Supported Features.

Objective: "Prevent performance regressions across a known set of representative models."
* Automated Performance Tests 
    * Define a set of Models to test covering a wide array of usecases and frameworks.
    * Publish performance results in a temporally comparable way.

Objective: "Improve the Serverless Experience by reducing cold starts/stops to 10 seconds on warmed models."
* Model Caching
    * Reduce model download time by caching models from cloud storage on Persistent Volumes.
* Image Caching
    * Reduce container download time by ensuring images are cached in all cloud environments.
* Server Shutdown
    * Ensure that all model servers shutdown within 10 seconds of not receiving traffic.

Objective: "Simplify User Experience."
* Secure storage mechanisms
    * Explore simplifying user experience with storage backends protected by credentials (e.g. S3/GCS accounts with credentials)

# Future 
## Unscheduled Work
* Multi-Model Serving.
    * Multiple KFServices share resources in the backend.
    * GPU Sharing.
* Flexible Inference Graphs [MLGraph CRD](https://github.com/SeldonIO/mlgraph).
    * Model Experimentation.
    * Ensembling.
    * Multi Arm Bandit.
* Bias, Skew, and Outlier Detection.
    * Online support in graph.
    * Offline support with Payload Logging.
* Meta-Protocol Definition.
    * Unify disparate protocols across frameworks.
* Adaptive batching support
    * Queue and batch requests to increase throughput.

# Historical
## Q2 2019
### v0.1: KFService Minimum Viable Product (ETA: June 30, 2019)
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