# Autoscaling

Knative Serving Revisions are automatically scaled up and down according
incoming traffic.

## Definitions

- Knative Serving **Revision** -- a custom resource which is a running snapshot
  of the user's code (in a Container) and configuration.
- Knative Serving **Route** -- a custom resource which exposes Revisions to
  clients via an Istio ingress rule.
- Kubernetes **Deployment** -- a k8s resource which manages the lifecycle of
  individual Pods running Containers. One of these is running user code in each
  Revision.
- Knative Serving **Autoscaler** -- another k8s Deployment running a single Pod
  which watches request load on the Pods running user code. It increases and
  decreases the size of the Deployment running the user code in order to
  compensate for higher or lower traffic load.
- Knative Serving **Activator** -- a k8s Deployment running a single,
  multi-tenant Pod (one per Cluster for all Revisions) which catches requests
  for Revisions with no Pods. It brings up Pods running user code (via the
  Revision controller) and forwards caught requests.
- **Concurrency** -- the number of requests currently being served at a given
  moment. More QPS or higher latency means more concurrent requests.

## Behavior

Revisions have three autoscaling states which are:

1. **Active** when they are actively serving requests,
1. **Reserve** when they are scaled down to 0 Pods but is still in service, and
1. **Retired** when they will no longer receive traffic.

When a Revision is actively serving requests it will increase and decrease the
number of Pods to maintain the desired average concurrent requests per Pod. When
requests are no longer being served, the Revision will be put in a Reserve
state. When the first request arrives, the Revision is put in an Active state,
and the request is queued until it becomes ready.

In the Active state, each Revision has a Deployment which maintains the desired
number of Pods. It also has an Autoscaler (one per Revision for single-tenancy;
one for all Revisions for multi-tenancy) which watches traffic metrics and
adjusts the Deployment's desired number of pods up and down. Each Pod reports
its number of concurrent requests each second to the Autoscaler.

In the Reserve state, the Revision has no scheduled Pods and consumes no CPU.
The Istio route rule for the Revision points to the single multi-tenant
Activator which will catch traffic for all Reserve Revisions. When the Activator
catches a request for a Reserve Revision, it will flip the Revision to an Active
state and then forward requests to the Revision when it ready.

In the Retired state, the Revision has provisioned resources. No requests will
be served for the Revision.

Note: Retired state is currently not set anywhere. See
[issue 1203](https://github.com/knative/serving/issues/1203).

## Context

The following diagram illustrates the mechanics of the autoscaler:

```diagram
   +---------------------+
   | ROUTE               |
   |                     |
   |   +-------------+   |
   |   | Istio Route |---------------+
   |   +-------------+   |           |
   |         |           |           |
   +---------|-----------+           |
             |                       |
             |                       |
             | inactive              | active
             |  route                | route
             |                       |
             |                       |
             |                +------|---------------------------------+
             V         watch  |      V                                 |
       +-----------+   first  |   +- ----+  create   +------------+    |
       | Activator |------------->| Pods |<----------| Deployment |<--------------+
       +-----------+          |   +------+           +------------+    |          |
             |                |       |                                |          | resize
             |   activate     |       |                                |          |
             +--------------->|       |                                |          |
                              |       |               metrics          |   +------------+
                              |       +----------------------------------->| Autoscaler |
                              |                                        |   +------------+
                              |                                        |
                              | REVISION                               |
                              +----------------------------------------+

```

## Design Goals

1. **Make it fast**. Revisions should be able to scale from 0 to 1000 concurrent
   requests in 30 seconds or less.
1. **Make it light**. Wherever possible the system should be able to figure out
   the right thing to do without the user's intervention or configuration.
1. **Make everything better**. Creating custom components is a short-term
   strategy to get something working now. The long-term strategy is to make the
   underlying components better so that custom code can be replaced with
   configuration. E.g. Autoscaler should be replaced with the K8s
   [Horizontal Pod Autoscaler](https://kubernetes.io/docs/tasks/run-application/horizontal-pod-autoscale/)
   and
   [Custom Metrics](https://kubernetes.io/docs/tasks/run-application/horizontal-pod-autoscale/#support-for-custom-metrics).

### Slow Brain / Fast Brain

The Knative Serving Autoscaler is split into two parts:

1. **Fast Brain** that maintains the desired level of concurrent requests per
   Pod (satisfying [Design Goal #1](#design-goals)), and the
1. **Slow Brain** that comes up with the desired level based on CPU, memory and
   latency statistics (satisfying [Design Goal #2](#design-goals)).

## Fast Brain Implementation

This is subject to change as the Knative Serving implementation changes.

### Code

- [Autoscaler Library](../../pkg/autoscaler/autoscaler.go)
- [Autoscaler Binary](../../cmd/autoscaler/main.go)
- [Queue Proxy Binary](../../cmd/queue/main.go)
- [Statistics Server](../../pkg/autoscaler/statserver/server.go)

### Autoscaler

There is a proxy in the Knative Serving Pods (`queue-proxy`) which is
responsible for enforcing request queue parameters (single or multi threaded),
and reporting concurrent client metrics to the Autoscaler. If we can get rid of
this and just use [Envoy](https://www.envoyproxy.io/docs/envoy/latest/), that
would be great (see [Design Goal #3](#design-goals)). The Knative Serving
controller injects the identity of the Revision into the queue proxy environment
variables. When the queue proxy wakes up, it will find the Autoscaler for the
Revision and establish a websocket connection. Every 1 second, the queue proxy
pushes a gob serialized struct with the observed number of concurrent requests
at that moment.

The Autoscaler runs a controller which monitors
["KPA"](../../pkg/apis/autoscaling/v1alpha1/pa_types.go) resources and monitors
and scales the embedded object reference via the `/scale` sub-resource.

The Autoscaler provides a websocket-enabled Statistics Server. Queue proxies
send their metrics to the Autoscaler's Statistics Server and the Autoscaler
maintains a 60-second sliding window of data points.

The Autoscaler implements a scaling algorithm with two modes of operation:
Stable Mode and Panic Mode.

#### Stable Mode

In Stable Mode the Autoscaler adjusts the size of the Deployment to achieve the
desired average concurrency per Pod (currently
[hardcoded](https://github.com/knative/serving/blob/c4a543ecce61f5cac96b0e334e57db305ff4bcb3/cmd/autoscaler/main.go#L36),
later provided by the Slow Brain). It calculates the observed concurrency per
pod by averaging all data points over the 60 second window. When it adjusts the
size of the Deployment it bases the desired Pod count on the number of observed
Pods in the metrics stream, not the number of Pods in the Deployment spec. This
is important to keep the Autoscaler from running away (there is delay between
when the Pod count is increased and when new Pods come online to serve requests
and provide a metrics stream).

#### Panic Mode

The Autoscaler evaluates its metrics every 2 seconds. In addition to the
60-second window, it also keeps a 6-second window (the panic window). If the
6-second average concurrency reaches 2 times the desired average, then the
Autoscaler transitions into Panic Mode. In Panic Mode the Autoscaler bases all
its decisions on the 6-second window, which makes it much more responsive to
sudden increases in traffic. Every 2 seconds it adjusts the size of the
Deployment to achieve the stable, desired average (or a maximum of 10 times the
current observed Pod count, whichever is smaller). To prevent rapid fluctuations
in the Pod count, the Autoscaler will only increase Deployment size during Panic
Mode, never decrease. 60 seconds after the last Panic Mode increase to the
Deployment size, the Autoscaler transitions back to Stable Mode and begins
evaluating the 60-second windows again.

#### Deactivation

When the Autoscaler has observed an average concurrency per pod of 0.0 for some
time ([#305](https://github.com/knative/serving/issues/305)), it will transition
the Revision into the Reserve state. This scales the Deployment to 0, stops any
single tenant Autoscaler associated with the Revision, and routes all traffic
for the Revision to the Activator.

### Activator

The Activator is a single multi-tenant component that catches traffic for all
Reserve Revisions. It is responsible for activating the Revisions and then
proxying the caught requests to the appropriate Pods. It woud be preferable to
have a hook in Istio to do this so we can get rid of the Activator (see
[Design Goal #3](#design-goals)). When the Activator gets a request for a
Reserve Revision, it calls the Knative Serving control plane to transition the
Revision to an Active state. It will take a few seconds for all the resources to
be provisioned, so more requests might arrive at the Activator in the meantime.
The Activator establishes a watch for Pods belonging to the target Revision.
Once the first Pod comes up, all enqueued requests are proxied to that Pod.
Concurrently, the Knative Serving control plane will update the Istio route
rules to take the Activator back out of the serving path.

## Slow Brain Implementation

_Currently the Slow Brain is not implemented and the desired concurrency level
is hardcoded at 1.0
([code](https://github.com/knative/serving/blob/01787a8cd70eab441d28872621bc26ad061bf14b/config/config-autoscaler.yaml#L47))._
