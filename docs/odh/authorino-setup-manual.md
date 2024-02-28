# Setting up Authorino to work with ODH KServe

This page guides you through the process of installing Authorino,
changing Service Mesh configuration to be able to use Authorino as an
authorization provider, and shows the needed configurations to let
ODH KServe work correctly.

## Prerequisites

* A configured instance of OpenShift Service Mesh is available in the cluster
* The [Authorino Operator](https://operatorhub.io/operator/authorino-operator) is already installed in the cluster
* The Open Data Hub operator is available in the cluster

## Creating an Authorino instance

The following steps will create a dedicated Authorino instance for usage in Open Data Hub.
We recommend that you don't share .

1. Create a namespace to install the Authorino instance:
    * `oc new-project opendatahub-auth-provider`
1. Enroll the namespace to the Service Mesh. Assuming your `ServiceMeshControlPlane` resource
   is in the `istio-system` namespace and named `data-science-smcp`, you would need to create
   the following resource:
    ```yaml
    apiVersion: maistra.io/v1
    kind: ServiceMeshMember
    metadata:
      name: default
      namespace: opendatahub-auth-provider
    spec:
      controlPlaneRef:
        namespace: istio-system
        name: data-science-smcp
    ```
1. Create the following `Authorino` resource:
    ```yaml
    apiVersion: operator.authorino.kuadrant.io/v1beta1
    kind: Authorino
    metadata:
      name: authorino
      namespace: opendatahub-auth-provider
    spec:
      authConfigLabelSelectors: security.opendatahub.io/authorization-group=default
      clusterWide: true
      listener:
        tls:
          enabled: false
      oidcServer:
        tls:
          enabled: false
    ```
1. Once Authorino is running, patch the Authorino deployment to inject the Istio
sidecar and make it part of the Service Mesh:
    * `oc patch deployment authorino -n opendatahub-auth-provider -p '{"spec": {"template":{"metadata":{"labels":{"sidecar.istio.io/inject":"true"}}}} }'`

## Prepare the Service Mesh to work with Authorino

Once the Authorino instance is configured, the `ServiceMeshControlPlane` resource
needs to be modified to configure Authorino as an authorization provider of the Mesh.

Assuming you have your `ServiceMeshControlPlane` resource in the `istio-system` namespace,
you named it `data-science-smcp`, and you followed the previous section without
renaming any resource, the following is a patch that you can apply with
`oc patch smcp --type merge -n istio-system`:

```yaml
spec:
  techPreview:
    meshConfig:
      extensionProviders:
      - name: opendatahub-auth-provider
        envoyExtAuthzGrpc:
          service: authorino-authorino-authorization.opendatahub-auth-provider.svc.cluster.local
          port: 50051
```

> [!IMPORTANT]
> You should apply this patch only if you don't have other extension providers. If you do,
> you should manually edit your `ServiceMeshControlPlane` resource and add the
> needed configuration.

## Configure KServe authorization

In ODH KServe, authorization is configured using a global Istio
`AuthorizationPolicy` targeting the predictor pods of InferenceServices. Also,
given the several hops of a request, an `EnvoyFilter` is used to reset the HTTP
_Host_ header to the original one of the inference request.

The following YAML are the resources that you need to apply to the namespace
where Service Mesh is installed, assuming you have followed the previous
instructions without renaming any resource. If you created your `ServiceMeshControlPlane`
resource in the `istio-system` namespace, you can create the resources using
an `oc apply -n istio-system` command.

```yaml
apiVersion: security.istio.io/v1beta1
kind: AuthorizationPolicy
metadata:
   name: kserve-predictor
spec:
   action: CUSTOM
   provider:
      name: opendatahub-auth-provider
   rules:
      - to:
           - operation:
                notPaths:
                   - /healthz
                   - /debug/pprof/
                   - /metrics
                   - /wait-for-drain
   selector:
      matchLabels:
         component: predictor
---
apiVersion: networking.istio.io/v1alpha3
kind: EnvoyFilter
metadata:
  name: activator-host-header
spec:
  priority: 20
  workloadSelector:
    labels:
      component: predictor
  configPatches:
  - applyTo: HTTP_FILTER
    match:
      listener:
        filterChain:
          filter:
            name: envoy.filters.network.http_connection_manager
    patch:
      operation: INSERT_BEFORE
      value:
        name: envoy.filters.http.lua
        typed_config:
          '@type': type.googleapis.com/envoy.extensions.filters.http.lua.v3.Lua
          inlineCode: |
           function envoy_on_request(request_handle)
              local headers = request_handle:headers()
              if not headers then
                return
              end

              local original_host = headers:get("k-original-host")
              if original_host then

                port_seperator = string.find(original_host, ":", 7)
                if port_seperator then
                  original_host = string.sub(original_host, 0, port_seperator-1)
                end
                headers:replace('host', original_host)
              end
            end
```
