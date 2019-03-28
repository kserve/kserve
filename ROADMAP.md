# KF Serving 2019 Roadmap
## Q2 2019

### Core CUJs
* High Level Interfaces
    * Deploy a Tensorflow model without specifying a Tensorflow Serving Technology.
    * Deploy a XGBoost model without specifying a XGBoost Serving Technology.
    * Deploy a ScikitLearn model without specifying a ScikitLearn Serving Technology.
    * Deploy a Custom Containerized model by specifying.

* Model Rollout
    * Rollout a model using a blue-green strategy.
    * Rollout a model using a pinned strategy.
    * Rollout a model using a canary strategy.

* Autoscaling 
    * Scale a model to zero.
    * Scale a model from zero without dropping traffic.
    * Scale a model that is GPU bound.
    * Scale a model that is CPU bound.

### High Level Work Items
* Define the API specification (owner ellisbigelow@)
    * Explain complete data model
    * Document common usage patterns to meet CUJs

* Implement the API specification with a CRD (owner yuzisun@)
    * Generate a Kubebuilder CRD
    * Define golang protos as per spec
    * Implement ValidatingAdmissionController for API Validation
    * Implement ReconciliationHandler to generate subresources

* Integrate a KFServing component with a SeldonDeployment (owner cliveseldon@)
    * Determine integration strategy
    * Implement integration

## Beyond Q2
TBD