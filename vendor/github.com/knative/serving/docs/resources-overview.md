# Resources Overview

This document provides a high-level description of the resources deployed to a
Kubernetes cluster in order to run Knative Serving. The exact list of resources
is going to change frequently during the current phase of active development. In
order to keep this document from becoming out-of-date frequently it doesn't
describe the exact individual resources but instead the higher level objects
which they form.

## Dependencies

Knative Serving depends on [Istio](https://istio.io/) in order to function.
Istio is responsible for setting up the network routing both inside the cluster
and ingress into the cluster.

## Components

There are four primary components to the Knative Serving system. The first is
the _Controller_ which is responsible for updating the state of the cluster
based on user input. The second is the _Webhook_ component which handles
validation of the objects and actions performed. The third is an _Activator_
component which brings back scaled-to-zero pods and forwards requests. The
fourth is the _Autoscaler_ which scales pods are requests come in.

The controller processes a series of state changes in order to move the system
from its current, actual state to the state desired by the user.

All of the Knative Serving components are deployed into the `knative-serving`
namespace. You can see the various objects in this namespace by running
`kubectl -n knative-serving get all`
([minus some admin-level resources like service accounts](https://github.com/kubernetes/kubectl/issues/151)).
To see only objects of a specific type, for example to see the webhook and
controller deployments inside Knative Serving, you can run
`kubectl -n knative-serving get deployments`.

The Knative Serving controller creates Kubernetes and Istio resources when
Knative Serving resources are created and updated. It will also create Build
resources when provided in the Configuration spec. These sub-resources will be
created in the same namespace as their parent Knative Serving resource, _not_
the `knative-serving` namespace. For example, if you create a Knative Serivce in
namespace 'foo' the corresponding Istio resources will also be in namespace
'foo'.

## Kubernetes Resource Configs

The various Kubernetes resource configurations are organized as follows:

```plain
# Knative Serving resources
config/*.yaml

# Istio release configuration
third_party/istio-*/install/kubernetes/...

# Knative Serving Monitoring configs (Optional)
config/monitoring/...

# Knative Build resources (Optional)
third_party/config/build/release.yaml

```

## Viewing resources after deploying Knative Serving

### Custom Resource Definitions

To view all of the custom resource definitions created, run
`kubectl get customresourcedefinitions`. These resources are named according to
their group, i.e. custom resources created by Knative Serving end with
`serving.knative.dev` or `internal.knative.dev`.

### Deployments

View the Knative Serving specific deployments by running
`kubectl -n knative-serving get deployments`. These deployments will ensure that
the correct number of pods are running for that specific deployment.

For example, given:

```console
$ kubectl -n knative-serving get deployments
NAME         DESIRED   CURRENT   UP-TO-DATE   AVAILABLE   AGE
activator    1         1         1            1           5d
autoscaler   1         1         1            1           5d
controller   1         1         1            1           5d
webhook      1         1         1            1           5d
```

Based on the desired state shown above, we expect there to be a single pod
running for each of the deployments shown above. We can verify this by running
and seeing similar output as shown below:

```console
$ kubectl -n knative-serving get pods
NAME                          READY     STATUS    RESTARTS   AGE
activator-c8495dc9-z7xpz      2/2       Running   0          5d
autoscaler-66897845df-t5cwg   2/2       Running   0          5d
controller-699fb46bb5-xhlkg   1/1       Running   0          5d
webhook-76b87b8459-tzj6r      1/1       Running   0          5d
```

Similarly, you can run the same commands in the istio (`istio-system`)
namespaces to view the running deployments. To view all namespaces, run
`kubectl get namespaces`.

### Service Accounts and RBAC policies

To view the service accounts configured for Knative Serving, run
`kubectl -n knative-serving get serviceaccounts`.

To view all cluster role bindings, run `kubectl get clusterrolebindings`.
Unfortunately there is currently no mechanism to fetch the cluster role bindings
that are tied to a service account.
