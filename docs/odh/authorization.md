# Protecting Inference Services under authorization

Starting Open Data Hub version 2.8, KServe is enhanced with request authorization
for `InferenceServices`. The protected services will require clients to provide
valid credentials in the HTTP Authorization request header. The provided credentials
must be valid, and must have enough privileges for the request to be accepted.

> [!NOTE]
> In ODH v2.8, the feature is broken and is fixed in ODH v2.9. 

## Setup

Authorization was implemented using [Istio's External Authorization
feature](https://istio.io/latest/docs/tasks/security/authorization/authz-custom/).
The chosen external authorizer is [Kuadrant's Authorino project](https://github.com/Kuadrant/authorino).

The Open Data Hub operator will deploy and manage an instance of Authorino. For
this, the [Authorino Operator](https://github.com/Kuadrant/authorino-operator) is
required to be installed in the cluster, which is [available in the
OperatorHub](https://operatorhub.io/operator/authorino-operator).

> [!NOTE]
> If you don't need authorization features, you can skip installing the Authorino
> Operator. The ODH operator will detect such situation and won't try to configure
> authorization capabilities.

Once you install Open Data Hub, you can use the [`DSCInitialization` sample
available in the opendatahub-operator repository](https://github.com/opendatahub-io/opendatahub-operator/blob/incubation/config/samples/dscinitialization_v1_dscinitialization.yaml):

```shell
oc apply -f https://github.com/opendatahub-io/opendatahub-operator/blob/incubation/config/samples/dscinitialization_v1_dscinitialization.yaml
```

After creating the `DSCInitialization` resource, the Open Data Hub operator should
deploy a Service Mesh instance, and an Authorino instance. Both components will
be configured to work together.

To deploy KServe, the `DataScienceCluster` resource required is the
following one:

```yaml
kind: DataScienceCluster
apiVersion: datasciencecluster.opendatahub.io/v1
metadata:
  name: default-dsc
spec:
  components:
    kserve:
      managementState: Managed
```

Notice that the provided `DataScienceCluster` only specifies the KServe component.
The fields for other components may get their default values, and you may end-up
with a quite complete ODH setup. If you need only KServe or a smaller set of
ODH components, use the [`DataScienceCluster` resource sample](https://github.com/opendatahub-io/opendatahub-operator/blob/incubation/config/samples/datasciencecluster_v1_datasciencecluster.yaml) and
modify it to fit your needs.

Once the `DataScienceCluster` resource is created, Knative serving will be installed
and configured to work with the Service Mesh and the Authorino instance that were
deployed via the `DSCInitialization` resource.

## Deploying a protected InferenceService

To demonstrate how to protect an `InferenceService`, a sample model generally
available from the upstream community will be used. The sample model is a
Scikit-learn model, and the following `ServingRuntime` needs to be created in
some namespace:

```yaml
apiVersion: serving.kserve.io/v1alpha1
kind: ServingRuntime
metadata:
  name: kserve-sklearnserver
spec:
  annotations:
    prometheus.kserve.io/port: '8080'
    prometheus.kserve.io/path: "/metrics"
    serving.knative.openshift.io/enablePassthrough: "true"
    sidecar.istio.io/inject: "true"
    sidecar.istio.io/rewriteAppHTTPProbers: "true"
  supportedModelFormats:
    - name: sklearn
      version: "1"
      autoSelect: true
      priority: 1
  protocolVersions:
    - v1
    - v2
  containers:
    - name: kserve-container
      image: docker.io/kserve/sklearnserver:latest
      args:
        - --model_name={{.Name}}
        - --model_dir=/mnt/models
        - --http_port=8080
      resources:
        requests:
          cpu: "1"
          memory: 2Gi
        limits:
          cpu: "1"
          memory: 2Gi
```

Then, deploy the sample model by creating the following `InferenceService` resource
in the same namespace as the previous `ServingRuntime`:

```yaml
apiVersion: "serving.kserve.io/v1beta1"
kind: "InferenceService"
metadata:
  name: "sklearn-v2-iris"
spec:
  predictor:
    model:
      modelFormat:
        name: sklearn
      protocolVersion: v2
      runtime: kserve-sklearnserver
      storageUri: "gs://kfserving-examples/models/sklearn/1.0/model"
```

The `InferenceService` still does not have authorization enabled. A sanity check
can be done by sending an unauthenticated request to the service, which should
reply as normally:

```bash
# Get the endpoint of the InferenceService
MODEL_ENDPOINT=$(kubectl get inferenceservice sklearn-v2-iris -o jsonpath='{.status.url}')
# Send an inference request:
curl -v \
  -H "Content-Type: application/json" \
  -d @./iris-input-v2.json \
  ${MODEL_ENDPOINT}/v2/models/sklearn-v2-iris/infer
```

You can download the `iris-input-v2.json` file from the following link:
[iris-input.json](https://github.com/opendatahub-io/kserve/blob/c146e06df7ea3907cd3702ed539b1da7885b616c/docs/samples/v1beta1/xgboost/iris-input.json)

If the sanity check is successful, the `InferenceService` is protected by
adding the `security.opendatahub.io/enable-auth=true` annotation:

```bash
oc annotate isvc sklearn-v2-iris security.opendatahub.io/enable-auth=true
```

The KServe controller will re-deploy the model. Once it is ready, the previous
`curl` request should be rejected because it is missing credentials. The credentials
are provided via the standard HTTP Authorization request header. The updated
`curl` request has the following form:

```bash
curl -v \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN"
  -d @./iris-input-v2.json \
  ${MODEL_ENDPOINT}/v2/models/sklearn-v2-iris/infer
```

You can provide any `$TOKEN` that is accepted by the OpenShift API server. The
request will only be accepted if the provided token has the `get` privilege over
`v1/Services` resources (core Kubernetes Services) in the namespace where
the `InferenceService` lives.
