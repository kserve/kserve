# KFServing Requirements for Kubeflow 1.0

- KFServing Authors: singhan@us.ibm.com
- Kubeflow Authors: chasm@google.com, jlewi@google.com, kam.d.kasravi@intel.com

## Objective

The purpose of this doc is to define requirements and recommendations aimed at graduating KFServing in Kubeflow to 1.0.

The main goals are to:

1. Establish criteria for graduating a KFServing to a supported Kubeflow application; i.e. 1.0
1. Set clear expectations for users regarding KFServing for Kubeflow 1.0

## Scope

This document is intended to cover KFServing deployed in the Kubernetes cluster as part of Kubeflow and in particular:

## Requirements

### Configuration and deployment

| Description | Category | Explanation |
|-------------|----------|-------------|
| Kustomize package  | Required  | <ul><li>Kubeflow has standardized on Kustomize for configuring and deploying packages </ul> |
| Application CR  | Required  | <ul><li>Kubeflow has standardized on using the Kubernetes Application CR to provide consistent metadata and monitoring about applications <li> Application CR should be an owner of all resources so that deleting the application CR uninstalls the application </ul> |
| ```app.kubernetes.io``` labels on every resource | Required  | <ul><li>Every resource that is part of the application should include the [labels](https://kubernetes.io/docs/concepts/overview/working-with-objects/common-labels/#labels) recommended by Kubernetes, currently: <ul><li>`app.kubernetes.io/name` <li>`app.kubernetes.io/instance` <li>`app.kubernetes.io/version` <li>`app.kubernetes.io/component` <li>`app.kubernetes.io/part-of` <li>`app.kubernetes.io/managed-by`</ul> <li>See example [here](https://github.com/kubeflow/manifests/blob/v0.6.1/tf-training/tf-job-operator/overlays/application/application.yaml#L8-L13) </ul> |
| Images listed in kustomization.yaml | Required  | <ul><li>All docker images used by KFServing should be listed in one or more kustomization.yaml files <li>This facilitates mirroring the images to different docker registries </ul> |
| Upgradeability | Required  | <ul><li>KFServing must support upgrading between consecutive major and minor releases </ul> |
| Separate cluster scoped and namespace scoped resources | Recommended  | <ul><li>To the extent possible cluster scoped resources should be installable separately (e.g. via a separate kustomize package) <li> This allows cluster admins to install only the cluster scoped resources <li> Clear documentation of cluster and namespace scoped behavior </ul> |
| Kustomize package should be deployable on its own | Recommended  | <ul><li>To the extent possible users should be able to run kustomize build | kubectl apply inside the kustomize package to deploy the application stand alone </ul> |

### Custom Resources

| Description | Category | Explanation |
|-------------|----------|-------------|
| Version stability | Required  | <ul><li>No deprecative API changes </ul> |
| Multi Version Support | Required  | <ul><li>Serves multiple versions of the [CR per K8s docs](https://kubernetes.io/docs/tasks/access-kubernetes-api/custom-resources/custom-resource-definition-versioning/#specify-multiple-versions) </ul> |
| Supports status subresource | Required  | <ul><li>Status subresource documented [here](https://kubernetes.io/docs/tasks/access-kubernetes-api/custom-resources/custom-resource-definitions/#status-subresource) </ul> |
| CRD schema validation | Required  | <ul><li>Validation documented [here](https://kubernetes.io/docs/tasks/access-kubernetes-api/custom-resources/custom-resource-definitions/#validation) <li>Example: [TFJob](https://github.com/kubeflow/kubeflow/blob/v0.6.1/kubeflow/tf-training/tf-job-operator.libsonnet#L81) </ul> |
| Training operators follow kubeflow/common conventions | Required  | <ul><li>Conventions defined [here](https://github.com/kubeflow/common/blob/master/job_controller/api/v1/types.go) <li>Examples: [TFJob](https://github.com/kubeflow/kubeflow/blob/v0.6.1/kubeflow/tf-training/tf-job-operator.libsonnet) and [PyTorchJob](https://github.com/kubeflow/kubeflow/blob/v0.6.1/kubeflow/pytorch-job/pytorch-operator.libsonnet) </ul> |

### Logging and monitoring

| Description | Category | Explanation |
|-------------|----------|-------------|
| Liveness/Readiness signals | Required  | <ul><li>KFServing should expose liveness and/or readiness signals as appropriate for the application </ul> |
| Prometheus metrics | Required  | <ul><li>KFServing should export suitable application level metrics (e.g. number of jobs) using prometheus  </ul> |
| Json logging | Recommended  | <ul><li>KFServing should optionally emit structured logs with suitable metadata to facilitate debugging <li>e.g. CR controllers should annotate log messages with the name of the resource involved so it's easy to filter logs to all messages about a resource </ul> |

### Docker Images

| Description | Category | Explanation |
|-------------|----------|-------------|
| Vulnerability Scanning | Required  | <ul><li>Docker images must be scanned for vulnerabilities and known vulnerabilities published </ul> |
| Licensing | Required  | <ul><li>Docker images must provide a list of all OSS licenses used by the image and its transitive dependencies  </ul> |

### CI/CD

| Description | Category | Explanation |
|-------------|----------|-------------|
| E2E tests | Required  | <ul><li>E2E tests should be run on presubmit, postsubmit, and periodically <li> Tests should cover deployment (kustomize packages) <li> As well as the application functionality </ul> |
| Scalability / load testing | Required  | <ul><li>Applications must have scalability and or load testing demonstrating that the application meets appropriate requirements for the application (e.g. # of concurrent CRs) </ul> |
| Continuous building of docker images | Recommended  | <ul><li>On post-submit, docker images should be automatically built and pushed  </ul> |
| Continuous updating of Kustomize manifests | Recommended  | <ul><li>On post-submit, kustomize packages should be automatically updated to point to latest images </ul> |

### Docs

| Description | Category | Explanation |
|-------------|----------|-------------|
| API Reference docs | Required  | <ul><li>Applications exposing APIs (e.g. CRs) need reference docs documenting the API </ul> |
| Application docs | Required  | <ul><li>There must be docs on kubeflow.org explaining what an application is used for and how to use it </ul> |


### Ownership/Maintenance

| Description | Category | Explanation |
|-------------|----------|-------------|
| Healthy number of committers and commits | Required  | <ul><li> Committers are listed as approvers in owners files <li> Number to be determined by TOC based on size and scope of application </ul> |
| At least 2 different organizations are committers | Required  | |

### Adoption

| Description | Category | Explanation |
|-------------|----------|-------------|
| List of users running the application | Recommended  | <ul><li>Suggest listing adopters willing to be identified publicly in ADOPTERS.md </ul> |

## Reference

* TFJob and PyTorch 1.0 [exit criterion](http://bit.ly/operators-exit-criterion)
* [CNCF Graduation Criteria](https://github.com/cncf/toc/blob/master/process/graduation_criteria.adoc)
* [Core Infrastructure Best Practices](https://github.com/coreinfrastructure/best-practices-badge)
* [Kubeflow v1.0 Docs and Processes](https://docs.google.com/document/d/1v06QmjIms3z-uoW-waS7S9IzMA6n4CVrXOwQQ9N0do4)
* [Application CR Issue](https://github.com/kubernetes-sigs/application/issues/6) related to dependencies and health monitoring 

