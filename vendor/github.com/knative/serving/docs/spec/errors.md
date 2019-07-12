# Error Conditions and Reporting

Knative Serving uses
[the standard Kubernetes API pattern](https://github.com/kubernetes/community/blob/master/contributors/devel/api-conventions.md#typical-status-properties)
for reporting configuration errors and current state of the system by writing
the report in the `status` section. There are two mechanisms commonly used in
`status`:

- **Conditions** represent true/false statements about the current state of the
  resource.

- **Other fields** may provide status on the most recently retrieved state of
  the system as it relates to the resource (example: number of replicas or
  traffic assignments).

Both of these mechanisms often include additional data from the controller such
as `observedGeneration` (to determine whether the controller has seen the latest
updates to the spec).

## Conditions

Conditions provide an easy mechanism for client user interfaces to indicate the
current state of resources to a user. Knative Serving resources should follow
[the k8s API conventions for `condition`](https://github.com/kubernetes/community/blob/master/contributors/devel/api-conventions.md#typical-status-properties)
and the patterns described in this section.

### Knative Serving condition `type`

Each resource should define a small number of success conditions as `type`s.
This should bias towards fewer than **5** high-level progress categories which
are separate and meaningful for customers. For a Revision, these might be
`BuildSucceeded`, `ResourcesAvailable` and `ContainerHealthy`.

Where it makes sense, resources should define a top-level "happy state"
condition `type` which indicates that the resource is set up correctly and ready
to serve.

- For long-running resources, this condition `type` should be `Ready`.
- For objects which run to completion, the condition `type` should be
  `Succeeded`.

### Knative Serving condition `status`

Each condition's `status` should be one of:

- `Unknown` when the controller is actively working to achieve the condition.
- `False` when the reconciliation has failed. This should be a terminal failure
  state until user action occurs.
- `True` when the reconciliation has succeeded. Once all transition conditions
  have succeeded, the "happy state" condition should be set to `True`.

Type names should be chosen such that these interpretations are clear:

- `BuildSucceeded` works because `True` = success and `False` = failure.
- `BuildCompleted` does not, because `False` could mean "in-progress".

Conditions may also be omitted entirely if it doesn't pertain to the resource at
hand. e.g. Revisions that take pre-build containers may elide Build-related
conditions.

Conditions with a status of `False` will also supply additional details about
the failure in
[the "Reason" and "Message" sections](#condition-reason-and-message).

### Knative Serving condition `reason` and `message`

The fields `reason` and `message` should be considered to have unlimited
cardinality, unlike [`type`](#condition-type) and [`status`](#condition-status).
If a resource has a "happy state" [`type`](#condition-type), it will surface the
`reason` and `message` from the first failing sub Condition.

The values `reason` takes on (while camelcase words) should be treated as
opaque. Clients shouldn't programmatically act on their values, but bias towards
using `reason` as a terse explanation of the state for end-users, whereas
`message` is the long-form of this.

## Example scenarios

Example user and system error scenarios are included below along with how the
status is presented to CLI and UI tools via the API.

- [Deployment-Related Failures](#deployment-related-failures)
  - [Revision failed to become Ready](#revision-failed-to-become-ready)
  - [Build failed](#build-failed)
  - [Resource exhausted while creating a revision](#resource-exhausted-while-creating-a-revision)
  - [Container image not present in repository](#container-image-not-present-in-repository)
  - [Container image fails at startup on Revision](#container-image-fails-at-startup-on-revision)
  - [Deployment progressing slowly/stuck](#deployment-progressing-slowly-stuck)
- [Routing-Related Failures](#routing-related-failures)
  - [Traffic not assigned](#traffic-not-assigned)
  - [Revision not found by Route](#revision-not-found-by-route)
  - [Configuration not found by Route](#configuration-not-found-by-route)
  - [Latest Revision of a Configuration deleted](#latest-revision-of-a-configuration-deleted)
  - [Traffic shift progressing slowly/stuck](#traffic-shift-progressing-slowly-stuck)
- [Scale-to-Zero](#scale-to-zero)
  - [Inactive Revision](#inactive-revision)
  - [Activating Revision](#activating-revision)
  - [Active Revision](#active-revision)

## Deployment-Related Failures

The following scenarios will generally occur when attempting to deploy changes
to the software stack by updating the Service or Configuration resources to
cause a new Revision to be created.

### Revision failed to become Ready

If the latest Revision fails to become `Ready` for any reason within some
reasonable timeframe, the Configuration and Service should signal this with the
`Ready` status and `ConfigurationsReady` status, respectively, copying the
reason and the message from the `Ready` condition on the Revision.

```http
GET /api/serving.knative.dev/v1alpha1/namespaces/default/configurations/my-service
```

```yaml
status:
  latestReadyRevisionName: abc
  latestCreatedRevisionName: bcd # Hasn't become "Ready"
  conditions:
    - type: Ready
      status: False
      reason: BuildFailed
      message: "Build Step XYZ failed with error message: $LASTLOGLINE"
```

```http
GET /api/serving.knative.dev/v1alpha1/namespaces/default/services/my-service
```

```yaml
status:
  latestReadyRevisionName: abc
  latestCreatedRevisionName: bcd # Hasn't become "Ready"
  conditions:
    - type: Ready
      status: False
      reason: BuildFailed
      message: "Build Step XYZ failed with error message: $LASTLOGLINE"
    - type: ConfigurationsReady
      status: False
      reason: BuildFailed
      message: "Build Step XYZ failed with error message: $LASTLOGLINE"
    - type: RoutesReady
      status: True
```

### Build failed

If the Build steps failed while creating a Revision, you can examine the
`Failed` condition on the Build or the `BuildSucceeded` condition on the
Revision (which copies the value from the build referenced by `spec.buildName`).
In addition, the Build resource (but not the Revision) should have a status
field to link to the log output of the build.

```http
GET /apis/build.knative.dev/v1alpha1/namespaces/default/builds/build-1acub3
```

```yaml
status:
  # Link to log stream; could be ELK or Stackdriver, for example
  buildLogsLink: "http://logging.infra.mycompany.com/...?filter=..."
  conditions:
    - type: Failed
      status: True
      reason: BuildStepFailed # could also be SourceMissing, etc
      message: "Step XYZ failed with error message: $LASTLOGLINE"
```

```http
GET /apis/serving.knative.dev/v1alpha1/namespaces/default/revisions/abc
```

```yaml
status:
  conditions:
    - type: Ready
      status: False
      reason: BuildFailed
      message: "Build Step XYZ failed with error message: $LASTLOGLINE"
    - type: BuildSucceeded
      status: False
      reason: BuildStepFailed
      message: "Step XYZ failed with error message: $LASTLOGLINE"
```

### Resource exhausted while creating a revision

Since a Revision is only metadata, the Revision will be created, but will have a
condition indicating the underlying failure, possibly indicating the failed
underlying resource. In a multitenant environment, the customer might not have
have access or visibility into the underlying resources in the hosting
environment.

```http
GET /apis/serving.knative.dev/v1alpha1/namespaces/default/revisions/abc
```

```yaml
status:
  conditions:
    - type: Ready
      status: False
      reason: NoDeployment
      message: "The controller could not create a deployment named abc-e13ac."
    - type: ResourcesProvisioned
      status: False
      reason: NoDeployment
      message: "The controller could not create a deployment named abc-e13ac."
```

### Container image not present in repository

Revisions might be created while a Build is still creating the container image
or uploading it to the repository. If the build is being performed by a CRD in
the cluster, the `spec.buildName` attribute will be set (and see the
[Build failed](#build-failed) example). In other cases when the build is not
supplied, the container image referenced might not be present in the registry
(either because of a typo or because it was deleted). In this case, the `Ready`
condition will be set to `False` with a reason of `ContainerMissing`. This
condition could be corrected if the image becomes available at a later time.
Knative Serving could also make a defensive copy of the container image to avoid
having to surface this error if the original docker image is deleted.

```http
GET /apis/serving.knative.dev/v1alpha1/namespaces/default/revisions/abc
```

```yaml
status:
  conditions:
    - type: Ready
      status: False
      reason: ContainerMissing
      message: "Unable to fetch image 'gcr.io/...': <literal error>"
    - type: ContainerHealthy
      status: False
      reason: ContainerMissing
      message: "Unable to fetch image 'gcr.io/...': <literal error>"
```

### Container image fails at startup on Revision

Particularly for development cases with interpreted languages like Node or
Python, syntax errors might only be caught at container startup time. For this
reason, implementations should start a copy of the container on deployment,
before marking the container `Ready`. If this container fails to start, the
`Ready` condition will be set to `False`, the reason will be set to `ExitCode%d`
with the exit code of the application, and the termination message from the
container will be provided. (Containers will be run with the default
`terminationMessagePath` and a `terminationMessagePolicy` of
`FallbackToLogsOnError`.) Additionally, the Revision `status.logsUrl` should be
present, which provides the address of an endpoint which can be used to fetch
the logs for the failed process.

```http
GET /apis/serving.knative.dev/v1alpha1/namespaces/default/revisions/abc
```

```yaml
status:
  logUrl: "http://logging.infra.mycompany.com/...?filter=revision_uid=a1e34&..."
  conditions:
    - type: Ready
      status: False
      reason: ExitCode127
      message: "Container failed with: SyntaxError: Unexpected identifier"
    - type: ContainerHealthy
      status: False
      reason: ExitCode127
      message: "Container failed with: SyntaxError: Unexpected identifier"
```

### Deployment progressing slowly/stuck

See
[the kubernetes documentation for how this is handled for Deployments](https://kubernetes.io/docs/concepts/workloads/controllers/deployment/#failed-deployment).
For Revisions, we will start by assuming a single timeout for deployment (rather
than configurable), and report that the Revision was not Ready, with a reason
`ProgressDeadlineExceeded`. Note that we will only report
`ProgressDeadlineExceeded` if we could not determine another reason (such as
quota failures, missing build, or container execution failures).

Since container setup time also affects the ability of 0 to 1 autoscaling, the
`Ready` failure with `ProgressDeadlineExceeded` reason should be considered a
terminal condition, even if Kubernetes might attempt to make progress even after
the deadline.

```http
GET /apis/serving.knative.dev/v1alpha1/namespaces/default/revisions/abc
```

```yaml
status:
  conditions:
    - type: Ready
      status: False
      reason: ProgressDeadlineExceeded
      message: "Did not pass readiness checks in 120 seconds."
```

## Routing-Related Failures

The following scenarios are most likely to occur when attempting to roll out a
change by shifting traffic to a new Revision. Some of these conditions can also
occur under normal operations due to (for example) operator error causing live
resources to be deleted.

### Traffic not assigned

If some percentage of traffic cannot be assigned to a live (materialized or
scaled-to-zero) Revision, the Route will report the `Ready` condition as
`False`. The Service will mirror this status in its' `Ready` condition. For
example, for a newly-created Service where the first Revision is unable to
serve:

```http
GET /apis/serving.knative.dev/v1alpha1/namespaces/default/routes/my-service
```

```yaml
status:
  domain: my-service.default.mydomain.com
  traffic:
    - revisionName: "Not found"
      percent: 100
  conditions:
    - type: Ready
      status: False
      reason: RevisionMissing
      message: "Configuration 'abc' does not have a LatestReadyRevision."
```

```http
GET /apis/serving.knative.dev/v1alpha1/namespaces/default/services/my-service
```

```yaml
status:
  latestCreatedRevisionname: abc
  # no latestReadyRevisionName, because abc failed
  domain: my-service.default.mydomain.com
  conditions:
    - type: Ready
      status: False
      reason: RevisionMissing
      message: "Configuration 'abc' does not have a LatestReadyRevision."
    - type: RoutesReady
      status: False
      reason: RevisionMissing
      message: "Configuration 'abc' does not have a LatestReadyRevision."
    - type: ConfigurationsReady
      status: True
```

### Revision not found by Route

If a Revision is referenced in a Route's `spec.traffic`, and the Revision cannot
be found, the `AllTrafficAssigned` condition will be marked as False with a
reason of `RevisionMissing`, and the Revision will be omitted from the Route's
`status.traffic`.

```http
GET /apis/serving.knative.dev/v1alpha1/namespaces/default/routes/my-service
```

```yaml
status:
  traffic:
    - revisionName: abc
      name: current
      percent: 100
  conditions:
    - type: Ready
      status: False
      reason: RevisionMissing
      message: "Revision 'qyzz' referenced in traffic not found"
    - type: AllTrafficAssigned
      status: False
      reason: RevisionMissing
      message: "Revision 'qyzz' referenced in traffic not found"
```

### Configuration not found by Route

If a Route references the `latestReadyRevisionName` of a Configuration and the
Configuration cannot be found, the `AllTrafficAssigned` condition will be marked
as False with a reason of `ConfigurationMissing`, and the Revision will be
omitted from the Route's `status.traffic`.

```http
GET /apis/serving.knative.dev/v1alpha1/namespaces/default/routes/my-service
```

```yaml
status:
  traffic: []
  conditions:
    - type: Ready
      status: False
      reason: ConfigurationMissing
      message: "Configuration 'abc' referenced in traffic not found"
    - type: AllTrafficAssigned
      status: False
      reason: ConfigurationMissing
      message: "Configuration 'abc' referenced in traffic not found"
```

### Latest Revision of a Configuration deleted

If the most recent Revision is deleted, the Configuration will set `Ready` to
False.

If the deleted Revision was also the most recent to become ready, the
Configuration will also clear the `latestReadyRevisionName`. Additionally, if
the Configuration in this case is referenced by a Route, the Route will set the
`AllTrafficAssigned` condition to False with reason `RevisionMissing`, as above.

```http
GET /apis/serving.knative.dev/v1alpha1/namespaces/default/configurations/my-service
```

```yaml
...
metadata:
  generation: 1234  # only updated when spec changes
  ...
spec:
  ...
status:
  latestCreatedRevision: abc
  conditions:
  - type: Ready
    status: False
    reason: RevisionMissing
    message: "The latest Revision appears to have been deleted."
  observedGeneration: 1234
```

### Traffic shift progressing slowly/stuck

Similar to deployment slowness, if the transfer of traffic (either via gradual
or abrupt rollout) takes longer than a certain timeout to complete/update, the
`RolloutInProgress` condition will remain at True, but the reason will be set to
`ProgressDeadlineExceeded`.

```http
GET /apis/serving.knative.dev/v1alpha1/namespaces/default/routes/my-service
```

```yaml
status:
  traffic:
    - revisionName: abc
      percent: 75
    - revisionName: def
      percent: 25
  conditions:
    - type: Ready
      status: False
      reason: ProgressDeadlineExceeded
      # reason is a short status, message provides error details
      message: "Unable to update traffic split for more than 120 seconds."
```

## Scale to Zero

The following scenarios outline the various circumstances surrounding scaling
the resources underlying a Revision to zero.

### Inactive Revision

When a Revision becomes inactive this is reflected by setting the `Active`
condition to `False`. Note that the Revision may stay Ready while it is scaled
to zero.

```http
GET /apis/serving.knative.dev/v1alpha1/namespaces/default/revisions/my-rev-00001
```

```yaml
status:
  conditions:
    - type: Ready
      status: True
    - type: Active
      status: False
      reason: NoTraffic
      severity: Info
      message: The target is not receiving traffic.
```

### Activating Revision

When an inactive Revision receives traffic, while traffic is buffered as
resources are provisioned, the Revision may reflect this by setting the `Active`
condition to `Unknown`.

```http
GET /apis/serving.knative.dev/v1alpha1/namespaces/default/revisions/my-rev-00001
```

```yaml
status:
  conditions:
    - type: Ready
      status: True
    - type: Active
      status: Unknown
      reason: Queued
      severity: Info
      message:
        Requests to the target are being buffered as resources are provisioned.
```

### Active Revision

When a Revision is actively receiving traffic, the Revision reflects this by
setting the `Active` condition to `True`.

```http
GET /apis/serving.knative.dev/v1alpha1/namespaces/default/revisions/my-rev-00001
```

```yaml
status:
  conditions:
    - type: Ready
      status: True
    - type: Active
      status: True
      severity: Info
```
