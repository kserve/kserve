# Knative Serving API spec

This file contains the [resource paths](#resource-paths) and
[yaml schemas](#resource-yaml-definitions) that make up the Knative Serving API.

## Resource Paths

Resource paths in the Knative Serving API have the following standard k8s form:

```http
/apis/{apiGroup}/{apiVersion}/namespaces/{metadata.namespace}/{kind}/{metadata.name}
```

For example:

```http
/apis/serving.knative.dev/v1alpha1/namespaces/default/routes/my-service
```

It is expected that each Route will provide a name within a cluster-wide DNS
name. While no particular URL scheme is mandated (consult the `domain` property
of the Route for the authoritative mapping), a common implementation would be to
use the kubernetes namespace mechanism to produce a URL like the following:

```http
[$revisionname].$route.$namespace.<common knative cluster suffix>
```

For example:

```http
prod.my-service.default.mydomain.com
```

## Resource YAML Definitions

YAMLs for the Knative Serving API resources are described below, describing the
basic k8s structure: metadata, spec and status, along with comments on specific
fields.

### Route

For a high-level description of Routes, [see the overview](overview.md#route).

```yaml
apiVersion: serving.knative.dev/v1alpha1
kind: Route
metadata:
  name: my-service
  namespace: default
  labels:
    knative.dev/service: ...  # name of the Service automatically filled in
  annotations:
    serving.knative.dev/creator: ...       # the user identity who created the service, system generated.
    serving.knative.dev/lastModifier: ...  # the user identity who last modified the service, system generated.

  # system generated meta
  uid: ...
  resourceVersion: ...  # used for optimistic concurrency control
  creationTimestamp: ...
  generation: ...  # updated only when spec changes; used by observedGeneration
  selfLink: ...
  ...
spec:
  traffic:
  # list of oneof configurationName | revisionName.
  #  configurationName watches configurations to address latest latestReadyRevisionName
  #  revisionName pins a specific revision
  - configurationName: ...
    name: ...  # +optional. Access as {name}.${status.domain},
               #  e.g. oss: current.my-service.default.mydomain.com
    percent: 100  # list percentages must add to 100. 0 is a valid list value
  - ...

status:
  # domain: The hostname used to access the default (traffic-split)
  #   route. Typically, this will be composed of the name and namespace
  #   along with a cluster-specific prefix (here, mydomain.com).
  domain: my-service.default.mydomain.com

  address: # knative/pkg/apis/duck/v1alpha1.Addressable
    # hostname: A DNS name for the default (traffic-split) route which can
    # be accessed without leaving the cluster environment.
    hostname: my-service.default.svc.cluster.local

  # DEPRECATED: see address.hostname (above)
  domainInternal: ...

  traffic:
  # current rollout status list. configurationName references
  #   are dereferenced to latest revision
  - revisionName: ...  # latestReadyRevisionName from a configurationName in spec
    name: ...
    percent: ...  # percentages add to 100. 0 is a valid list value
    url: ... # present when name is set. URL of the named traffic target
  - ...

  conditions:  # See also the [error conditions documentation](errors.md)
  - type: Ready
    status: True
  - type: AllTrafficAssigned
    status: True
  - ...

  observedGeneration: ...  # last generation being reconciled
```

### Configuration

For a high-level description of Configurations,
[see the overview](overview.md#configuration).

```yaml
apiVersion: serving.knative.dev/v1alpha1
kind: Configuration
metadata:
  name: my-service
  namespace: default
  labels:
    knative.dev/service: ...  # name of the Service automatically filled in
    knative.dev/route: ...  # name of the Route automatically filled in
  # system generated meta
  uid: ...
  resourceVersion: ...  # used for optimistic concurrency control
  creationTimestamp: ...
  generation: ...  # updated only when spec changes; used by observedGeneration
  selfLink: ...
  ...
spec:
  # +optional. a complete definition of the build resource to create.
  # For back-compat, this may also be a build.knative.dev/v1alpha1.BuildSpec
  build:
    # This inlines an example with knative/build, but may be any kubernetes
    # resource that completes with a Succeeded condition as outlined in our
    # errors.md
    apiVersion: build.knative.dev
    kind: Build
    metadata:
      annotations: ...
      labels: ...
    spec: ...

  revisionTemplate: ... # Deprecated way of expressing the same as template.

  template:  # template for building Revision
    metadata:
      # A name may be optionally specified, but it must:
      #  * be unique within the namespace,
      #  * have the Configuration name as a prefix,
      #  * be changed or removed on any subsequent revisionTemplate updates.
      name: ...
    spec:  # knative.RevisionTemplateSpec. Copied to a new revision

      # +optional. DEPRECATED, use buildRef
      buildName: ...

      # +optional. if rolling back, the client may set this to the
      #   previous revision's build to avoid triggering a rebuild
      buildRef:  # corev1.ObjectReference
        apiVersion: build.knative.dev/v1alpha1
        kind: Build
        name: foo-bar-00001

      # is a core.v1.Container; some fields not allowed, such as resources, ports
      container: ... # See the Container section below

      # is a heavily restricted []core.v1.Volume; only the secret and configMap
      # types are allowed.
      volumes:
      - name: foo
        secret: ...
      - name: bar
        configMap: ...

      # +optional concurrency strategy.  Defaults to Multi.
      # Deprecated in favor of ContainerConcurrency.
      concurrencyModel: ...
      # +optional max request concurrency per instance.  Defaults to `0` (system decides)
      # when concurrencyModel is unspecified as well.  Defaults to `1` when
      # concurrencyModel `Single` is provided.
      containerConcurrency: ...
      # +optional. max time the instance is allowed for responding to a request
      timeoutSeconds: ...
      serviceAccountName: ...  # Name of the service account the code should run as.

status:
  # the latest created and ready to serve. Watched by Route
  latestReadyRevisionName: abc
  # latest created revision, may still be in the process of being materialized
  latestCreatedRevisionName: def
  conditions:  # See also the [error conditions documentation](errors.md)
  - type: Ready
    status: False
    reason: ContainerMissing
    message: "Unable to start because container is missing and build failed."
  observedGeneration: ...  # last generation being reconciled
```

### Revision

For a high-level description of Revisions,
[see the overview](overview.md#revision).

```yaml
apiVersion: serving.knative.dev/v1alpha1
kind: Revision
metadata:
  name: myservice-a1e34  # system generated
  namespace: default
  labels:
    knative.dev/configuration: ...  # name of the Configuration automatically filled in
    knative.dev/service: ...  # name of the Service automatically filled in
    knative.dev/configurationGeneration: ... # generation of configuration that created this Revision
  # system generated meta
  uid: ...
  resourceVersion: ...  # used for optimistic concurrency control
  creationTimestamp: ...
  generation: ...
  selfLink: ...
  ...

# spec populated by Configuration
spec:
  # +optional. DEPRECATED use buildRef
  buildName: ...

  # +optional. a reference to the Kubernetes resource that is responsible
  # for producing this Revision's image. This resource is expected to
  # terminate with a "Succeeded" condition as outlined in errors.md
  buildRef:  # corev1.ObjectReference
    apiVersion: build.knative.dev/v1alpha1
    kind: Build
    name: foo-bar-00001

  container: ... # See the Container section below

  # is a heavily restricted []core.v1.Volume; only the secret and configMap
  # types are allowed.
  volumes:
  - name: foo
    secret: ...
  - name: bar
    configMap: ...

  # Name of the service account the code should run as.
  serviceAccountName: ...

  # Deprecated and not updated anymore
  # Used to be the Revision's level of readiness for receiving traffic.
  servingState: Active | Reserve | Retired

  # Some function or server frameworks or application code may be
  # written to expect that each request will be granted a single-tenant
  # process to run (i.e. that the request code is run
  # single-threaded). An containerConcurrency value of `1` will
  # guarantee that only one request is handled at a time by a given
  # instance of the Revision container. A value of `2` or more will
  # limit request concurrency to that value.  A value of `0` means the
  # system should decide.
  containerConcurrency: 0 | 1 | 2-N
  # Deprecated in favor of containerConcurrency
  concurrencyModel: Single | Multi

  # Many higher-level systems impose a per-request response deadline.
  timeoutSeconds: ...

status:
  # This is a copy of metadata from the container image or grafeas,
  # indicating the provenance of the revision. This is based on the
  # container image, but may need further clarification.
  imageSource:
    git|gcs: ...

  conditions:  # See also the documentation in errors.md
   - type: Ready
     status: False
     message: "Starting Instances"
  # If spec.buildRef is provided
  - type: BuildSucceeded
    status: True
  - ...

  # URL for accessing the logs generated by this specific revision.
  # Note that logs may still be access controlled separately from
  # access to the API object.
  logUrl: "http://logging.infra.mycompany.com/...?filter=revision_uid=a1e34&..."

  # serviceName: The name for the core Kubernetes Service that fronts this
  #   revision. Typically, the name will be the same as the name of the
  #   revision.
  serviceName: myservice-a1e34

  # imageDigest: The imageDigest is the spec.container.image field resolved
  #   to a particular digest at revision creation.
  imageDigest: gcr.io/my-project/...@sha256:60ab5...
```

## Service

For a high-level description of Services,
[see the overview](overview.md#service).

```yaml
apiVersion: serving.knative.dev/v1alpha1
kind: Service
metadata:
  name: myservice
  namespace: default
  # system generated meta
  uid: ...
  resourceVersion: ...  # used for optimistic concurrency control
  creationTimestamp: ...
  generation: ...
  selfLink: ...
  ...

# spec contains one of several possible rollout styles
spec:  # One of "runLatest", "release", "pinned" (DEPRECATED), or "manual"

  # Example, only one of "runLatest", "release", "pinned" (DEPRECATED), or "manual" can be set in practice.
  runLatest:
    configuration:  # serving.knative.dev/v1alpha1.ConfigurationSpec
      # +optional. The build resource to instantiate to produce the container.
      build: ...
      revisionTemplate: ... # Deprecated way of expressing the same as template.
      template:
        metadata:
          # Name may be specified here following the same rules as Configuration.
          # When the name collides, the Route must not be updated to the
          # conflicting Revision.
        spec: # serving.knative.dev/v1alpha1.RevisionSpec
          container: ... # See the Container section below
          volumes: ... # Optional
          containerConcurrency: ... # Optional
          timeoutSeconds: ... # Optional
          serviceAccountName: ...  # Name of the service account the code should run as

  # Example, only one of "runLatest", "release", "pinned" (DEPRECATED), or "manual" can be set in practice.
  pinned:
    revisionName: myservice-00013  # Auto-generated revision name
    configuration:  # serving.knative.dev/v1alpha1.ConfigurationSpec
      # +optional. The build resource to instantiate to produce the container.
      build: ...
      revisionTemplate: ... # Deprecated way of expressing the same as template.
      template:
        spec: # serving.knative.dev/v1alpha1.RevisionSpec
          container: ... # See the Container section below
          volumes: ... # Optional
          containerConcurrency: ... # Optional
          timeoutSeconds: ... # Optional
          serviceAccountName: ...  # Name of the service account the code should run as

  # Example, only one of "runLatest", "release", "pinned" (DEPRECATED), or "manual" can be set in practice.
  release:
    # Ordered list of 1 or 2 revisions. First revision is traffic target
    # "current" and second revision is traffic target "candidate".
    # It is possible to specify `"@latest"` string as a shortcut to the lastest available revision.
    revisions: ["myservice-00013", "myservice-00015"]
    rolloutPercent: 50 # Percent [0-99] of traffic to route to "candidate" revision
    configuration:  # serving.knative.dev/v1alpha1.ConfigurationSpec
      # +optional. The build resource to instantiate to produce the container.
      build: ...
      revisionTemplate: ... # Deprecated way of expressing the same as template.
      template:
        metadata:
          # Name may be specified here following the same rules as Configuration.
          # When the name collides, the Route must not be updated to the
          # conflicting Revision.
        spec: # serving.knative.dev/v1alpha1.RevisionSpec
          container: ... # See the Container section below
          volumes: ... # Optional
          containerConcurrency: ... # Optional
          timeoutSeconds: ... # Optional
          serviceAccountName: ...  # Name of the service account the code should run as

  # Example, only one of "runLatest", "release", "pinned" (DEPRECATED), or "manual" can be set in practice.
  # Manual has no fields. It enables direct access to modify a previously created
  # Route and Configuration
  manual: {}
status:
  # This information is copied from the owned Configuration and Route.

  # The latest created and ready to serve Revision.
  latestReadyRevisionName: abc
  # Latest created Revision, may still be in the process of being materialized.
  latestCreatedRevisionName: def

  # domain: The hostname used to access the default (traffic-split)
  #   route. Typically, this will be composed of the name and namespace
  #   along with a cluster-specific prefix (here, mydomain.com).
  domain: myservice.default.mydomain.com

  address: # knative/pkg/apis/duck/v1alpha1.Addressable
    # hostname: A DNS name for the default (traffic-split) route which can
    # be accessed without leaving the cluster environment.
    hostname: myservice.default.svc.cluster.local

  # DEPRECATED: see address.hostname (above)
  domainInternal: ...

  # current rollout status list. configurationName references
  #   are dereferenced to latest revision
  traffic:
  - revisionName: ...  # latestReadyRevisionName from a configurationName in spec
    name: ...
    percent: ...  # percentages add to 100. 0 is a valid list value
    url: ... # present when name is set. URL of the named traffic target
  - ...

  conditions:  # See also the documentation in errors.md
  - type: Ready
    status: False
    reason: RevisionMissing
    message: "Revision 'qyzz' referenced in traffic not found"
  - type: ConfigurationsReady
    status: False
    reason: ContainerMissing
    message: "Unable to start because container is missing failed."
  - type: RoutesReady
    status: False
    reason: RevisionMissing
    message: "Revision 'qyzz' referenced in traffic not found"

  observedGeneration: ...  # last generation being reconciled
```

## Container

This is a
[core.v1.Container](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.11/#container-v1-core).
Fields not specified in the container object below are disallowed. Examples of
disallowed fields include `name` and `lifecycle`.

This type is not used on its own but is found composed inside
[Service](#service), [Configuration](#configuration), and [Revision](#revision).

Items with `...` behave as specified in the Kubernetes API documentation unless
otherwise noted.

```yaml
container: # v1.Container
  args: [...] # Optional
  command: [...] # Optional
  env: ... # Optional
  envFrom: ... # Optional

  # image is either provided as pre-built container, or built by Knative Serving from
  # source. When built by Knative, this should match what the Build is directed to produce, e.g.
  # build.template.arguments[_IMAGE], as the "promise" of a future build.
  # If buildRef is provided, it is expected that this image will be
  # present when the referenced build is complete.
  image: gcr.io/... # Required
  imagePullPolicy: ... # Optional

  # HTTPGetAction and TCPSocketAction are the supported probe options.
  livenessProbe: # Optional
    failureThreshold: ...
    httpGet: ...
    initialDelaySeconds: ...
    periodSeconds: ...
    successThreshold: ...
    tcpSocket: ...
    timeoutSeconds: ...

  # Only a single containerPort may be specified.
  # This can be specified to select a specific port for incoming traffic.
  # This is useful if your application cannot discover the port to listen
  # on through the $PORT environment variable that is always set within the container.
  # Some fields are not allowed, such as hostIP and hostPort.
  ports: # Optional
    # Valid range is [1-65535], except 8012 (RequestQueuePort)
    # and 8022 (RequestQueueAdminPort).
    - containerPort: ...
      name: ... # Optional, one of "http1", "h2c"
      protocol: ... # Optional, one of "", "tcp"

  # HTTPGetAction and TCPSocketAction are the supported probe options.
  readinessProbe: ... # Optional
    failureThreshold: ...
    httpGet: ...
    initialDelaySeconds: ...
    periodSeconds: ...
    successThreshold: ...
    tcpSocket: ...
    timeoutSeconds: ...

  resources: ... # Optional
  securityContext: ... # Optional
  terminationMessagePath: ... # Optional
  terminationMessagePolicy: ... # Optional

  # This specifies where the volumes present in the RevisionSpec will be mounted into
  # the container.  Each of the volumes in the RevisionSpec must be mounted here
  # or it is an error.
  volumeMounts: # Optional
    - name: ... # This must match a name from Volumes
      mountPath: ... # Where to mount the named Volume.
      readOnly: ... # Must be True, will default to True, so it may be omitted.
  workingDir: ... # Optional
```
