# KServe Roadmap
## 2021 Q4/2022 Q1

### Kubernetes Deployment
Objective: "Enable raw kubernetes deployment as alternative mode"
* Support existing ML frameworks, transformer/explainer, logger and batching
* Make Istio/KNative optional and unlock KNative limitations
  * Allow multiple volumes mounted
  * Allow TCP/UDP

### Inference Graph
Objective: "Enable model serving pipelines with flexible routing graph"
* Inference Router
    * Model Experimentation.
    * Ensembling.
    * Multi Arm Bandit.
    * Pipeline
Proposal: https://docs.google.com/document/d/1rV8kI_40oiv8jMhY_LwkkyKLdOwSI1Qda-Dc6Dgjz1g

### ModelMesh
Objective: "Unifying interface for SingleModel and ModelMesh deployment"
* Ability to perform inference using Predict v2 API with REST/gRPC
* Unify the storage support for single and ModelMesh
* InferenceService controller to utilize ServingRuntime
* Single install for KServe which includes SingleModel and ModelMesh Serving
