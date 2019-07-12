# Knative Runtime Contract

## Abstract

The Knative serverless compute infrastructure extends the
[Open Container Initiative Runtime Specification](https://github.com/opencontainers/runtime-spec/blob/master/spec.md)
to describe the functionality and requirements for serverless execution
workloads. In contrast to general-purpose containers, stateless
request-triggered (i.e. on-demand) autoscaled containers have the following
properties:

- Little or no long-term runtime state (especially in cases where code may be
  scaled to zero in the absence of request traffic).
- Logging and monitoring aggregation (telemetry) is important for understanding
  and debugging the system, as containers may be created or deleted at any time
  in response to autoscaling.
- Multitenancy is highly desirable to allow cost sharing for bursty applications
  on relatively stable underlying hardware resources.

This contract does not define the control surfaces over the runtime environment
except by [reference to the Knative Kubernetes resources](spec/spec.md).
Similarly, this contract does not define the implementation of metrics or
logging aggregation, except to provide a contract for the collection of logging
data. It is expected that access to the aggregated telemetry will be provided by
the platform operator.

## Background

The key words "MUST", "MUST NOT", "REQUIRED", "SHALL", "SHALL NOT", "SHOULD",
"SHOULD NOT", "RECOMMENDED", "NOT RECOMMENDED", "MAY", and "OPTIONAL" are to be
interpreted as described in [RFC 2119](https://tools.ietf.org/html/rfc2119).

The
[OCI specification](https://github.com/opencontainers/runtime-spec/blob/master/spec.md)
([v1.0.1](https://github.com/opencontainers/runtime-spec/blob/v1.0.1/spec.md))
is the basis for this document. When this document and the OCI specification
conflict, this document is assumed to override the general OCI recommendations.
Where this document does not specify behavior, developers should assume an OCI
compliant underlying implementation. Additionally, the core Knative definition
assumes the
[Linux Container Configuration](https://github.com/opencontainers/runtime-spec/blob/master/config-linux.md).

In particular, the default Knative implementation relies on Kubernetes behavior
to implement container operation. In some cases, current Kubernetes behavior in
2018 is not as performant as recommended in this documentation. The goal of the
Knative authors is to push as much of the required functionality into Kubernetes
and/or Istio as possible, rather than implementing reach-around layers.

This document considers two users of a given Knative environment, and is
particularly concerned with the expectations of _developers_ (and _language and
tooling developers_, by extension) running code in the environment.

- **Developers** write code which is packaged into a container which is run on
  the Knative cluster.
  - **Language and tooling developers** typically write tools used by
    _developers_ to package code into containers. As such, they are concerned
    that tooling which wraps developer code complies with this runtime contract.
- **Operators** (also known as **platform providers**) provision the compute
  resources and manage the software configuration of Knative and the underlying
  abstractions (for example, Linux, Kubernetes, Istio, etc).

## Runtime and Lifecycle

Knative aims to minimize the amount of tuning and production configuration
required to run a service. Some of these production-friendly features include:

1.  Stateless computation at request-scale or event-scale granularity.
1.  Automatic scaling between 0 and many instances (the process scale-out
    model).
1.  Automatic adjustment of resource requirements based on observed behavior,
    where possible.

In order to achieve these properties, containers which are operated as part of a
serverless platform SHOULD observe the following properties:

- Fast startup time (<1s until a request or event can be processed, given
  container image layer caching),
- Minimize local state (in support of autoscaling and scale to zero),
- CPU usage only while requests are active (see
  [this issue](https://github.com/knative/serving/issues/848) for reasons an
  operator might want to de-allocate CPU between requests).

### State

The general OCI state may not be available for introspection within the
container, and may only be visible to the system operator or platform provider.
In a highly-shared environment, containers may experience the following:

- Containers with `status` of `stopped` MAY be immediately reclaimed by the
  system.
- The container process MAY be started as pid 0, through the use of PID
  namespaces or other processes.

### Lifecycle

- The container MAY be killed when the container is inactive. Serverless
  computation relies on inbound requests to determine container activeness. The
  container is sent a `SIGTERM` signal when it is killed via the
  [OCI specification's `kill`](https://github.com/opencontainers/runtime-spec/blob/master/runtime.md#kill)
  command to allow for a graceful shutdown of existing resources and
  connections. If the container has not shut down after a defined grace period,
  the container is forcibly killed via a `SIGKILL` signal.
- The environment MAY restrict the use of `prestart`, `poststart`, and
  `poststop` hooks to platform operators rather than developers. All of these
  hooks are defined in the context of the runtime namespace, rather than the
  container namespace, and may expose system-level information (and are
  non-portable).
- Failures of the developer-specified process MUST be logged to a
  developer-visible logging system.

In addition, some serverless environments may use an execution model other than
docker in linux (for example, [runv](https://github.com/hyperhq/runv) or
[Kata Containers](https://katacontainers.io/)). Implementations using an
execution model beyond docker in linux MAY alter the lifecycle contract beyond
the OCI specification as long as:

1.  An OCI-compliant lifecycle contract is the default, regardless of how many
    extensions are provided.
1.  The implementation of an extended execution model or lifecycle MUST provide
    documentation about the extended model or lifecycle **and** documentation
    about how to opt in to the extended lifecycle contract.

### Errors

- Platforms MAY provide mechanisms for post-mortem viewing of filesystem
  contents from a particular execution. Because containers (particularly failing
  containers) can experience frequent starts, operators or platform providers
  SHOULD limit the total space consumed by these failures.
- A container should write its own termination message to `/dev/termination-log`
  by default. If no message is written by the container, the last few lines of
  log output should be reported as the execution error (i.e. by
  [setting the `terminationMessagePolicy` to `FallbackToLogsOnError`](https://kubernetes.io/docs/tasks/debug-application-cluster/determine-reason-pod-failure/#customizing-the-termination-message))
  on Kubernetes.

### Warnings

As specified by OCI.

### Operations

[The OCI interface](https://github.com/opencontainers/runtime-spec/blob/v1.0.0-rc3/runtime.md#operations)
SHOULD NOT be exposed within the container. The operator or platform provider
MAY have the ability to directly interact with the OCI interface, but that is
beyond the scope of this specification.

An OPTIONAL method of invoking the `kill` operation MAY be exposed to developers
to provide signalling to the container.

### Hooks

Operation hooks SHOULD NOT be configurable by the Knative developer. Operators
or platform providers MAY use hooks to implement their own lifecycle controls.

### Linux Runtime

#### File descriptors

A read from the `stdin` file descriptor on the container SHOULD always result in
`EOF`. The `stdout` and `stderr` file descriptors on the container SHOULD be
collected and retained in a developer-accessible logging repository.
(TODO:[docs#902](https://github.com/knative/docs/issues/902)).

Within the container, pipes and file descriptors may be used to communicate
between processes running in the same container.

#### Dev symbolic links

As specified by OCI.

## Network Environment

For request-response functions, 0->many scaling is enabled by control of the
inbound request path to enable capturing and stalling inbound requests until an
autoscaled container is available to serve that request.

### Inbound network connectivity

Inbound network connectivity is assumed to use HTTP/1.1 compatible transport, to
enable the runtime environment to determine whether a container is "busy" or not
for purposes of scaling CPU and removing idle containers.

#### Protocols and Ports

The container MUST accept HTTP/1.1 requests from the environment. The
environment
[SHOULD offer an HTTP/2.0 upgrade option](https://http2.github.io/http2-spec/#discover-http)
(`Upgrade: h2c` on either the initial request or an `OPTIONS` request) on the
same port as HTTP/1.1. The developer MAY specify this port at deployment; if the
developer does not specify a port, the platform provider MUST provide a default.
Only one inbound `containerPort` SHALL be specified in the
[`core.v1.Container`](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.10/#containerport-v1-core)
specification. The `hostPort` parameter SHOULD NOT be set by the developer or
the platform provider, as it can interfere with ingress autoscaling. Regardless
of its source, the selected port will be made available in the `PORT`
environment variable.

The platform provider SHOULD configure the platform to perform HTTPS termination
and protocol transformation e.g. between QUIC or HTTP/2 and HTTP/1.1. Developers
should not need to implement multiple transports between the platform and their
code. Unless overridden by setting the
[`name`](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.10/#containerport-v1-core)
field on the inbound port, the platform will perform automatic detection as
described above. If the
[`core.v1.Container.ports[0].name`](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.10/#containerport-v1-core)
is set to one of the following values, HTTP negotiation will be disabled and the
following protocol will be used:

- `http1`: HTTP/1.1 transport and will not attempt to upgrade to h2c..
- `h2c`: HTTP/2 transport, as described in
  [section 3.4 of the HTTP2 spec (Starting HTTP/2 with Prior Knowledge)](https://http2.github.io/http2-spec/#known-http)

Developers SHOULD prefer to use automatic content negotiation where available,
and MUST NOT set the `name` field to arbitrary values, as additional transports
may be defined in the future. Developers MUST assume all traffic is
intermediated by an L7 proxy. Developers MUST NOT assume a direct network
connection between their server process and client processes.

#### Headers

As requests to the container will be proxied by the platform, all inbound
request headers SHOULD be set to the same values as the incoming request. Some
implementations MAY strip certain HTTP headers for security or other reasons;
such implementations SHOULD document the set of stripped headers. Because the
full set of HTTP headers is constantly evolving, it is RECOMMENDED that
platforms which strip headers define a common prefix which covers all headers
removed by the platform.

In addition, the following base set of HTTP/1.1 headers MUST be set on the
request:

- `Host` - As specified by
  [RFC 7230 Section 5.4](https://tools.ietf.org/html/rfc7230#section-5.4)

Also, the following proxy-specific request headers MUST be set:

- `Forwarded` - As specified by [RFC 7239](https://tools.ietf.org/html/rfc7239).

Additionally, the following legacy headers SHOULD be set for compatibility with
client software:

- `X-Forwarded-For`
- `X-Forwarded-Proto`

In addition, the following headers SHOULD be set to enable tracing and
observability features:

- Trace headers - Platform providers SHOULD provide and document headers needed
  to propagate trace contexts,
  [in the absence of w3c standardization](https://www.w3.org/2018/04/distributed-tracing-wg-charter.html).

Operators and platform providers MAY provide additional headers to provide
environment specific information.

#### Meta Requests

The
[`core.v1.Container`](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.10/#container-v1-core)
object allows specifying both a
[`readinessProbe`](https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-probes/#define-readiness-probes)
and a
[`livenessProbe`](https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-probes/#define-a-liveness-http-request).
If not provided, container startup and listening on the declared HTTP socket is
considered sufficient to declare the container "ready" and "live" (see the probe
definition below). If specified, liveness and readiness probes are REQUIRED to
be of the `httpGet` or `tcpSocket` types, and MUST target the inbound container
port; platform providers SHOULD disallow other probe methods.

Because serverless platforms automatically scale instances based on inbound
requests, and because noncompliant (or even failing) containers may be provided
by developers, the following defaults SHOULD be applied by the platform provider
if not set by the developer. The probes are intended to be trivially supportable
by naive conforming containers while preventing interference with developer
code. These settings apply to both `livenessProbe` and `readinessProbe`:

- `tcpSocket` set to the container's port
- `initialDelaySeconds` set to 0
- `periodSeconds` set to platform-specific value

In order to enable scaling in response to load, developers SHOULD NOT set the
`initialDelaySeconds` to a value greater than 0, and should aim to minimize
container startup time (aka cold start time).

##### Deployment probe

On the initial deployment, platform providers SHOULD start an instance of the
container to validate that the container is valid and will become ready. This
startup SHOULD occur even if the container would not serve any user requests. If
a container cannot satisfy the `readinessProbe` during deployment startup, the
Revision SHOULD be marked as failed.

Initial readiness probes allow the platform to avoid attempting to
later provision or scale deployments (Revisions) which cannot become healthy,
and act as a backstop to developer testing (via CI/CD or otherwise) which has
been performed on the supplied container. Common causes of these failures can
include: malformed dynamic code not tested in the container, environment
differences between testing and deployment environment, and missing or
misconfigured backends. This also provides an opportunity for the container to
be run at least once despite scale-to-zero guarantees.

### Outbound network connectivity

OCI does not specify any properties of the network environment in which a
container runs. The following items are OPTIONAL additions to the runtime
contract which describe services which may be of particular value to platform
providers.

#### DNS

Platform providers SHOULD override the DNS related configuration files under
`/etc` to enable local DNS lookups in the target environment (see
[Default Filesystems](#default-filesystems)).

#### Metadata Services

Platform providers MAY provide a network service to provide introspection and
environment information to the running process. Such a network service SHOULD be
an HTTP server with an operator- or provider-defined URL schema. If a metadata
service is provided, the schema MUST be documented. Sample use cases for such
metadata include:

- Container information or control interfaces.
- Host information, including maintenance or capability information.
- Access to external configuration stores (such as the Kubernetes ConfigMap
  APIs).
- Access to secrets or identity tokens, to enable key rotation.

## Configuration

### Root

Platform providers MAY set the `readonly` bit on the container to `true` in
order to reduce the possible disk space provisioning and management of
serverless workloads. Containers MUST use the provided temporary storage areas
(see [Default Filesystems](#default-filesystems)) for working files and caches.

### Mounts

In general, stateless applications should package their dependencies within the
container and not rely on mutable external state for templates, logging
configuration, etc. In some cases, it may be necessary for certain application
settings to be overridden at deploy time (for example, database backends or
authentication credentials). When these settings need to be loaded via a file,
read-only mounts of application configuration and secrets are supported by
`ConfigMap` and `Secrets` volumes. Platform providers MAY apply updates to
`Secrets` and `ConfigMaps` while the application is running; these updates could
complicate rollout and rollback. It is up to the developer to choose appropriate
policies for mounting and updating `ConfigMap` and `Secrets` which are mounted
as volumes.

As serverless applications are expected to scale horizontally and statelessly,
per-container volumes are likely to introduce state and scaling bottlenecks and
are NOT RECOMMENDED.

### Process

Serverless applications which scale horizontally are expected to be managed in a
declarative fashion, and individual instances SHOULD NOT be interacted with or
connected directly.

- The `terminal` property SHOULD NOT be set to `true`.
- The linux process specific properties MUST NOT be configurable by the
  developer, and MAY set by the operator or platform provider.

The following environment variables MUST be set:

| Name   | Meaning                                                                                                                                             |
| ------ | --------------------------------------------------------------------------------------------------------------------------------------------------- |
| `PORT` | Ingress `containerPort` for ingress requests and health checks. See [Inbound network connectivity](#inbound-network-connectivity) for more details. |

The following environment variables SHOULD be set:

| Name              | Meaning                                                                                                          |
| ----------------- | ---------------------------------------------------------------------------------------------------------------- |
| `K_REVISION`      | Name of the current Revision.                                                                                    |
| `K_CONFIGURATION` | Name of the Configuration that created the current Revision.                                                     |
| `K_SERVICE`       | If the current Revision has been created by manipulating a Knative Service object, name of this Knative Service. |

Platform providers MAY set additional environment variables. Standardization of
such variables will follow demonstrated usage and utility.

### User

Developers MAY specify that containers should be run as a specific user or group
ID using the `runAsUser` container property. If specified, the runtime MUST run
the container as the specified user ID if allowed by the platform (see below).
If no `runAsUser` is specified, a platform-specific default SHALL be used.
Platform Providers SHOULD document this default behavior.

Operators and Platform Providers MAY prohibit certain user IDs, such as `root`,
from executing code. In this case, if the identity selected by the developer is
invalid, the container execution MUST be failed.

### Default Filesystems

The OCI specification describes a default container environment which may be
used for many different purposes, including containerization of existing legacy
or stateful processes which may store substantial amounts of on-disk state. In a
scaled-out, stateless environment, container startup and teardown is accelerated
when on-disk resources are kept to a minimum. Additionally, developers may not
have access to the container filesystems (or the containers may be rapidly
recycled), so log aggregation SHOULD be provided.

In addition to the filesystems recommended in the OCI, the following filesystems
MUST be provided:

| Mount      | Description                                                                                                                                                      |
| ---------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `/tmp`     | MUST be Read-write.<p>SHOULD be backed by tmpfs if disk load is a concern.                                                                                       |
| `/var/log` | MUST be a directory with write permissions for logs storage. Implementations MAY permit the creation of additional subdirectories and log rotation and renaming. |
| `/dev/log` | MUST be a writable socket to syslog                                                                                                                              |

In addition, the following files may be overridden by the runtime environment to
enable DNS resolution:

| File               | Description                                                                                                                                                          |
| ------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `/etc/hosts`       | MAY be overridden to provide host mappings for well-known or provider-specific resources.                                                                            |
| `/etc/hostname`    | OPTIONAL: some environments may set this to a different value for each container, but other environments may use the same value for all containers.                  |
| `/etc/resolv.conf` | SHOULD be set to a valid cluster-specific recursive resolver. Providers MAY provide additional default search domains to improve customer experience in the cluster. |

Platform providers MAY provide additional platform-specific mount points
(example: shared read-only object stores or DB connection brokers). If provided,
the location and contents of the mount points SHOULD be documented by the
platform provider.

### Namespaces

The namespace configuration MUST be provided by the operator or platform
provider; developers or container providers MUST NOT set or assume a particular
namespace configuration.

### Devices

Developers MUST NOT use OCI `devices` to request additional devices beyond the
[OCI specification "Default Devices"](https://github.com/opencontainers/runtime-spec/blob/master/config-linux.md#default-devices).

### Control Groups

Control group (cgroups) controllers MUST be selected and configured by the
operator or platform provider. The cgroup devices SHOULD be mounted as
read-only.

#### Memory and CPU limits

The serverless platform MAY automatically adjust the resource limits (e.g. CPU)
based on observed resource usage. The limits enforced to a container SHOULD be
exposed in

- `/sys/fs/cgroup/memory/memory.limit_in_bytes`
- `/sys/fs/cgroup/cpu/cpu.cfs_period_us`
- `/sys/fs/cgroup/cpu/cpu.cfs_quota_us`

Additionally, operators or the platform MAY restrict or prevent CPU scheduling
for instances when no requests are active,
[where this capability is available](https://github.com/knative/serving/issues/848).
The Knative authors are currently discussing the best implementations options
for this feature with the Kubernetes SIG-Node team.

### Sysctl

The sysctl parameter applies system-wide kernel parameter tuning, which could
interfere with other workloads on the host system. This is not appropriate for a
shared environment, and SHOULD NOT be exposed for developer tuning.

### Seccomp

Seccomp provides a mechanism for further restricting the set of linux syscalls
permitted to the processes running inside the container environment. A seccomp
sandbox MAY be enforced by the platform operator; any such application profiles
SHOULD be configured and applied in a consistent mechanism outside of the
container specification. As the seccomp policy may be part of the platform
security hardening, operators MAY tune this over time as the threat environment
changes.

### Rootfs Mount Propagation

From the OCI spec:

> `rootfsPropagation` (string, OPTIONAL) sets the rootfs's mount propagation.
> Its value is either slave, private, shared or unbindable. The
> [Shared Subtrees](https://www.kernel.org/doc/Documentation/filesystems/sharedsubtree.txt)
> article in the kernel documentation has more information about mount
> propagation.

This option should only be set by the operator or platform provider, and MUST
NOT be configurable by the developer. As mount propagation may be part of the
platform security hardening, operators MAY tune this over time as the threat
environment changes.

### Masked Paths

This option MAY only be set by the operator or platform provider, and MUST NOT
be configurable by the developer. As masked paths may be part of the platform
security hardening, operators may tune this from time to time as the threat
environment changes.

### Readonly Paths

This option MAY only be set by the operator or platform provider, and MUST NOT
be configurable by the developer.

### Posix-platform Hooks

Operation hooks SHOULD NOT be configurable by the developer. Operators or
platform providers MAY use hooks to implement their own lifecycle controls.

### Annotations

As specified by OCI.
