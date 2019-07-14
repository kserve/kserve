# Resource Types

The primary resources in the Knative Serving API are Routes, Revisions,
Configurations, and Services:

- A **Route** provides a named endpoint and a mechanism for routing traffic to

- **Revisions**, which are immutable snapshots of code + config, created by a

- **Configuration**, which acts as a stream of environments for Revisions.

- **Service** acts as a top-level container for managing the set of Routes and
  Configurations which implement a network service.

![Object model](images/object_model.png)

## Route

**Route** provides a network endpoint for a user's service (which consists of a
series of software and configuration Revisions over time). A kubernetes
namespace can have multiple routes. The route provides a long-lived, stable,
named, HTTP-addressable endpoint that is backed by one or more **Revisions**.
The default configuration is for the route to automatically route traffic to the
latest revision created by a **Configuration**. For more complex scenarios, the
API supports splitting traffic on a percentage basis, and CI tools could
maintain multiple configurations for a single route (e.g. "golden path" and
“experiments”) or reference multiple revisions directly to pin revisions during
an incremental rollout and n-way traffic split. The route can optionally assign
addressable subdomains to any or all backing revisions.

## Revision

**Revision** is an immutable snapshot of code and configuration. A revision
references a container image, and optionally a build that is responsible for
materializing that container image from source. Revisions are created by updates
to a **Configuration**.

Revisions that are not addressable via a Route will be _retired_ and all
underlying K8s resources will be deleted. This provides a lightweight history of
the revisions a configuration has produced over time, and enables users to
easily rollback to a prior revision.

Revisions that are addressable via a Route will have resource utilization
proportional to the load they are under.

## Configuration

A **Configuration** describes the desired latest Revision state, and creates and
tracks the status of Revisions as the desired state is updated. A configuration
might include instructions on how to transform a source package (either git repo
or archive) into a container by referencing a
[Build](https://github.com/knative/build), or might simply reference a container
image and associated execution metadata needed by the Revision. On updates to a
Configuration, a new build and/or deployment (creating a Revision) may be
performed; the Configuration's controller will track the status of created
Revisions and makes both the most recently created and most recently _ready_
(i.e. healthy) Revision available in the status section.

## Service

A **Service** encapsulates a set of **Routes** and **Configurations** which
together provide a software component. Service exists to provide a singular
abstraction which can be access controlled, reasoned about, and which
encapsulates software lifecycle decisions such as rollout policy and team
resource ownership. Service acts only as an orchestrator of the underlying
Routes and Configurations (much as a kubernetes Deployment orchestrates
ReplicaSets), and its usage is optional but recommended.

The Service's controller will track the statuses of its owned Configuration and
Route, reflecting their statuses and conditions as its own.

The owned Configurations' Ready conditions are surfaced as the Service's
ConfigurationsReady condition. The owned Routes' Ready conditions are surfaced
as the Service's RoutesReady condition.

## Orchestration

The system will be configured to disallow users from creating
([NYI](https://github.com/knative/serving/issues/664)) or changing Revisions.
Instead, Revisions are created indirectly when a Configuration is created or
updated. This provides:

- a single referenceable resource for the route to perform automated rollouts
- a single resource that can be watched to see a history of all the revisions
  created
- PATCH semantics for revisions implemented server-side, minimizing
  read-modify-write implemented across multiple clients, which could result in
  optimistic concurrency errors
- the ability to rollback to a known good configuration

In the conventional single live revision scenario, a service creates both a
route and a configuration with the same name as the service. Update operations
on the service enable scenarios such as:

- _"Push code, keep config":_ Specifying a new revision with updated source,
  inheriting configuration such as env vars from the configuration.
- _"Update config, keep code"_: Specifying a new revision as just a change to
  configuration, such as updating an env variable, inheriting all other
  configuration and source/image.
- _"Execute a manual rollout"_: Updating the service when in pinned rollout mode
  allows manual testing of a revision before making it live.

Using a Service object to orchestrate the creation a both route and
configuration allows deployment of code (e.g. from a github button) to avoid
needing to reason about sequencing and failure modes of parallel resource
creation. The [sample API usage](normative_examples.md) section illustrates
conventional usage of the API.
