# KF Serving Roadmap
## 2021
### Multi Model Serving Phase Two
Objective: "Make multi model serving production ready"
* Trained model status probing and propagate CRD status
* Memory based Trained model sharding
* Scalability and Performance testing

Proposal: https://docs.google.com/document/d/1D_SF_RpMbItnupjnIlGazPmq9yzVd4RzP1yDfOK0KdY/edit

### Kubernetes Deployment
Objective: "Enable raw kubernetes deployment as alternative mode"
* Support existing ML frameworks
* Unlock knative limitations

### Inference Graph
Objective: "Enable model serving pipelines with flexible routing graph"
* Inference Router
    * Model Experimentation.
    * Ensembling.
    * Multi Arm Bandit.
    * Pipeline
 
Proposal: https://docs.google.com/document/d/1rV8kI_40oiv8jMhY_LwkkyKLdOwSI1Qda-Dc6Dgjz1g

### Batch Prediction

# Historical
### v0.5 API Stabilization and TCO Reduction(Jan, 2021)
Objective:  "Stabilize KFServing API"
* KFServing v1beta1 API
    * Promote v1alpha2 to v1beta1
    * Conversion webhook

Objective: "Unify prediction protocols across model servers"
* KFServing [prediction V2 API]([prediction V2 API](https://github.com/kubeflow/kfserving/tree/master/docs/predict-api/v2))
    * V2 KFServing Python Server(SKLearn/XgBoost/Custom)
    * Triton inference server V2 prediction API
    * TorchServe/KFServing integration
    * Enable support for GRPC.

Proposal: https://docs.google.com/document/d/1C2uf4SaAtwLTlBCciOhvdiKQ2Eay4U72VxAD4bXe7iU

Objective: "Reduce Total Cost of Ownership when deploying multiple underutilized models."
* Container/GPU Sharing
    * Reduce TCO by enabling models of the same framework and version to be co-hosted in a single model server.

Proposal: https://docs.google.com/document/d/11qETyR--oOIquQke-DCaLsZY75vT1hRu21PesSUDy7o

### v0.4 Performance(Oct, 2020)
Objective: "Prevent performance regressions across a known set of representative models."
* Automated Performance Tests
    * Define a set of Models to test covering a wide array of use cases and frameworks.
    * Publish performance results over time to enable regression tracking.

Objective: "Increase throughput for the inference service"
* Adaptive batching support
    * Queue and batch requests to increase throughput.

### v0.3 Stability (Mar 11, 2020)
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
