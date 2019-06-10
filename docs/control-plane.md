# Control Plane Specification
## KFService
A KFService is a unit of model serving. Users may have many KFServices, including different definitions across dev, test, and prod environments, as well as across different model use cases. KFServices are defined using the [Kubernetes Custom Resources](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/) standard. 

## Specifying a Service
### Resource Definition
This top level resource definition is shared by all Kubernetes Custom Resources.

| Field       |  Value      | Description |
| ----------- | ----------- | ----------- |
| kind       | KFService                     | [Read the Docs](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#types-kinds) |
| apiVersion | serving.kubeflow.org/v1alpha1 | [Read the Docs](https://kubernetes.io/docs/reference/using-api/api-overview/#api-versioning) |
| metadata   | [Metadata](#Metadata)         | [Read the Docs](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#metadata) |
| spec       | [Spec](#Spec)                 | [Read the Docs](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#spec-and-status) |
| status     | [Status](#Status)             | [Read the Docs](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#spec-and-status) |

### Metadata
The Metadata section of the resource definition contains resource identification fields and specialized flags. Name and namespace are immutable and if changed, will result in the creation of a new resource alongside the old one. Label and annotations are mutable. Labels and annotations will be applied to all relevant subresources of the Service enabling users to apply the same grouping at the pod level and pass though any necessary annotations.

| Field       | Value       | Description |
| ----------- | ----------- | ----------- |
| metadata.name        | String              | A name for your KFService. This will ultimately derive the internal and external URIs. |
| metadata.namespace   | String              | A namespace for your Service. This will ultimately derive the internal and external URIs. If missing, defaults to the namespace "default". |
| metadata.labels      | Map<String, String> | A set of key value pairs denoting arbitrary information (e.g integrate with MLOps Lifecycle systems or organize many Models). |
| metadata.annotations | Map<String, String> | A set of key value pairs used to enable specific features available in different kubernetes environments (e.g., Google Cloud ML Engine vs AzureML). |

### Spec
The Spec section of the resource definition encapsulates the desired state of the user's model serving resources. Changes made to the spec will be enacted upon the underlying servers in an eventually consistent manner. This infrastructure-as-code paradigm enables the coveted GitOps design pattern, where configurations are checked into git and may be reverted and applied to restore previous configurations. All fields are mutable and may be applied idempotently.

| Field       | Value       | Description |
| ----------- | ----------- | ----------- |
| spec.default               | [ModelSpec](#ModelSpec) | The default traffic route serving a ModelSpec. |
| spec.canary                | [ModelSpec](#ModelSpec) | An optional traffic route serving a percent of traffic. |
| spec.canaryTrafficPercent  | Integer                 | The amount of traffic to sent to the canary, defaults to 0. |

### Status
The Status section of the resource definition provides critical feedback to the user about the current state of the resource. A typical user workflow would include modifying the Spec and then monitoring the Status until it reflects the desired changes. Any dynamic information such as replica scale or error conditions will appear in the spec.

| Field       | Value       | Description |
| ----------- | ----------- | ----------- |
| status.url          | String                          | The url for accessing the Service. |
| status.default      | StatusConfigurationSpec         | The status of the configuration including name, replicas, and traffic. |
| status.canary       | StatusConfigurationSpec         | The status of the configuration including name, replicas, and traffic. |
| status.conditions   | List\<[Condition](#Conditions)> | The name for your Service. This will ultimately derive DNS name. |

## Understanding The Data Model
KFService offers a few high level specifications for common ML technologies. These come out of the box with a performant model server and a low flexibility, low barrier to entry experience. For more complex use cases, users are encouraged to take advantage of the custom spec, which allows a user's container to take advantage of the KFService architecture while providing the flexibility needed for their use case. 

### ModelSpec
| Field       | Value       | Description |
| ----------- | ----------- | ----------- |
| tensorflow  | [Tensorflow](#Tensorflow)   | A high level specification for Tensorflow models. |
| xgboost     | [XGBoost](#XGBoost)         | A high level specification for XGBoost models. |
| scikitlearn | [ScikitLearn](#ScikitLearn) | A high level specification for ScikitLearn models. |
| pytorch     | [Pytorch](#Pytorch)         | A high level specification for Pytorch models. |
| custom      | [Custom](#Custom)           | A flexible custom specification for arbitrary customer provided containers. |
| minReplicas | Integer                     | An optional integer specifying the minimum scale of the default/canary model definition. |
| maxReplicas | Integer                     | An optional integer specifying the maximum scale of the default/canary model definition. |

### Rollouts
The traffic management rules are specifically targetting Rollout use cases. A/B Testing, Multi-Armed-Bandit, and Ensemble Inferencing should not be implemented using KFService's default/canary mechanism. More complex controllers for these features should be purpose built and operate at a higher level than a KFService, perhaps even implemented using multiple KFServices or a KFGraph. This decoupling is critical to enable features like canarying a change to a Multi-Armed-Bandit inferencing graph.

#### Blue Green Rollout
The simpliest deployment strategy relies on KFService's default spec. The rollout workflow is identical to a Kubernetes Deployment. Changes to the CRD will create a new set of serving pods. Once the resources are available for serving, traffic will be flipped to the new resources. Users may observe the status of the KFService to gain insight into the current state of the Blue Green rollout.

#### Pinned Rollout
For more cautious and manual workflows, users may follow the Pinned Release pattern. It is executed using the following:

* Start with a KFService that has a "stable" or "golden" default definition.
* Modify the KFService to set the new ModelSpec under the `canary`.
* Set canaryTrafficPercent to 0
* Observe the KFService until the new revision progresses
    * Note: traffic is still sent to "default"
* Execute any inspection or testing against `canary`
    * Note: The canary will be available under a specialized URI (e.g. canary.name.namespace.cluster.internal)
* Once complete, change the key `canary` to `default` and delete the original `default` section.
* The underlying resource will swap traffic which can be observed in `status`

#### Canary Rollout
For the most advanced deployments, users may follow the Canary Rollout Pattern. It is executed using the following:

* Start with a KFService that has a "stable" or "golden" default definition.
* Modify the KFService to set the new ModelSpec under the `canary`.
* Set canaryTrafficPercent to some integer value (e.g. 10)
* Observe the KFService until the new revision progresses
    * `canaryTrafficPercent` is sent to `canary`
    * `100 - canaryTrafficPercent` is sent to `default`
* Continue incrementing canaryTrafficPercent in accordance with your desired canarying strategy
* Once complete, change the key `canary` to `default` and delete the original `default` section and `canaryTrafficPercent`.
* The underlying resource will swap traffic which can be observed in `status`

#### Rollback
One of the most compelling characteristics of "infrastructure-as-code" interfaces like the Kubernetes Resource Model is the ability to revert a configuration to a last known golden state. The default/canary pattern defines the entire current state, enabling GitOps rollback.

* revert to previously safe commit
* kubectl apply KFService.yaml

It's possible to revert to a commit that contains `canary` in the spec. The state will be reconciled to rollback to a configuration that runs both `default` and `canary`. For a traditional rollback workflow, we recommend reverting to KFService specs that contains only `default`.

### Resources
The default implementation will attempt to provide sane defaults for CPU and MEMORY. If more control is needed, we defer to [Specifying Resources in Kubernetes](https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container).

By default, a Service will be configured with requests and limits of:
* 1 CPU
* 2 GB MEM

In addition to specifying nvidia/gpu count, users may target specific GPUs on specific platforms by using annotations (e.g. "cloud.google.com/gke-accelerator": "nvidia-tesla-t4"):

### Replicas
KFService is a serverless platform. By default, replicas will scale to zero, and replicas will be dynamically generated based on request volume. For those familiar with Knative Services, KFService uses a concurrency of 1 as it's scale metric.

Some use cases are not tolerant to the limitations of auto-scaling. To accommodate these use cases, it is possible to specify `spec.[default,canary].minReplicas` and `spec.[default,canary].maxReplicas`.
It is important to note that this may result in large resource usage during rollout if `minReplicas` is set to a large value on both default and canary.

Replicas will be surfaced as part of `status.canary` and `status.default`.

```
status:
  default:
    ready: true
    replicas: 9
    traffic: 90
  canary:
    ready: true
    replicas: 1
    traffic: 10
```

### Conditions
Conditions provide realtime feedback to users on the underlying state of their deployment. As the feature set of KFService grows, additional conditions will be added to the set. We will use the convention of positive-true conditions; the resource is healthy if there are no conditions whose status is False. If no conditions are present on the resource (e.g. Ready is not present), the resource is not healthy. Only one condition for each type can be available.  

| Type        | Description |
| ----------- | ----------- | 
| Ready                | Becomes True when everything is ready. |
| ContainersHealthy    | Becomes True when containers are successfully pulled, started, and pass readinessChecks. |
| ResourcesProvisioned | Becomes True when minReplicas matches provisionedReplias. If provisioning error detected, reports False immediately. |

### Tensorflow
### ModelSpec
| Field       | Value       | Description |
| ----------- | ----------- | ----------- |
| modelUri       | String                                                                                             | URI pointing to Saved Model assets. KFService supports loading Saved Model assets from [PersistentVolumeClaim (PVC)](https://kubernetes.io/docs/concepts/storage/persistent-volumes/#persistentvolumeclaims). To mount the PVC with containers, user needs to define PVC name as `pvcName` and and mount path as`pvcMountPath` in [metadata.annotations](#Metadata).|
| runtimeVersion | String                                                                                             | Defaults to latest the version of Tensorflow. |
| resources      | [Resources](https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/) | Defaults to requests and limits of 1CPU, 2Gb MEM. |

### XGBoost
Currently, this is identical to [Tensorflow](#Tensorflow)

### ScikitLearn
Currently, this is identical to [Tensorflow](#Tensorflow)

### PyTorch
Currently, this is identical to [Tensorflow](#Tensorflow)

### Custom
TODO Elaborate
