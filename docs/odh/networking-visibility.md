# Configuring services as public or private

The InferenceServices and InferenceGraphs can be configured as public or
private. The private services are only reachable from within the cluster. The
public services are accessible by clients external to the cluster.

## Configuring InferenceServices as public or private

In ODH project, the default network visibility of InferenceServices depends on
its deployment mode:
* InferenceServices deployed in **Serverless** mode are public by default
* InferenceServices deployed in **Raw** mode are private by default. Notice this
  is different from the upstream KServe project which configures Raw
  InferenceServices as public by default.

Public InferenceServices in _Serverless_ mode are exposed via OpenShift Routes
that are created in the namespace of the Service Mesh Control Plane, which is
usually the `istio-system` namespace.

An InferenceService deployed in **Serverless** mode can be configured as private
by adding the `networking.knative.dev/visibility=cluster-local` label to the
InferenceService. The following is a YAML snippet showing how to configure the
label:

```yaml
apiVersion: serving.kserve.io/v1beta1
kind: InferenceService     
metadata:
  labels:
    networking.knative.dev/visibility: cluster-local
```

You can also run `oc label isvc ${your_isvc_name}
networking.knative.dev/visibility=cluster-local` to add the label to an existing
InferenceService in _Serverless_ mode to reconfigure it as private.

InferenceServices deployed in **Raw** mode use a different label. By adding the
`networking.kserve.io/visibility=exposed` label, the InferenceService will be
configured as public. The following is a YAML snippet showing how to configure
the label:

```yaml
apiVersion: serving.kserve.io/v1beta1
kind: InferenceService     
metadata:
  labels:
    networking.kserve.io/visibility: exposed
```

You can also run `oc label isvc ${your_isvc_name}
networking.kserve.io/visibility=exposed` to add the label to an existing
InferenceService in _Raw_ mode to reconfigure it as private.

Public InferenceServices in _Raw_ mode are exposed via OpenShift Routes
that are created in the same namespace as the InferenceService.

## Configuring InferenceGraphs as public or private

In ODH project, the default network visibility of InferenceGraphs is _public_
regardless of the deployment mode.

Similarly to InferenceServices, the InferenceGraphs that are deployed in
**Serverless** mode can be configured as private by adding the
`networking.knative.dev/visibility=cluster-local` label to the resource.

InferenceGraphs that are deployed in **Raw** mode, currently, do not offer a way
for switching to private. This is work in progress.
