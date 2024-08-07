# KServe on OpenShift Container Platform

[OpenShift Container Platform](https://www.openshift.com/products/container-platform) is built on top of Kubernetes, and offers a consistent hybrid cloud foundation for building and scaling containerized applications.

# Contents

1. [Installation](#installation)
2. [Test KServe installation](#test-kserve-installation)

# Installation

**Note**: These instructions were tested on OpenShift 4.12, with KServe 0.10

KServe can use Knative as a model serving layer. In OpenShift Knative comes bundled as a product called [OpenShift Serverless](https://docs.openshift.com/container-platform/latest/serverless/about/about-serverless.html). OpenShift Serverless can be installed with two different ingress layers:

* [Kourier](https://github.com/knative-sandbox/net-kourier)
* [OpenShift Service Mesh (Istio)](https://www.redhat.com/en/technologies/cloud-computing/openshift/what-is-openshift-service-mesh)

Follow the corresponding installation guide:

1. [Kourier](#installation-with-kourier)
2. [Service Mesh](#installation-with-service-mesh)

## Installation with Kourier

```bash
# Install OpenShift Serverless operator
oc apply -f openshift/serverless/operator.yaml
oc wait --for=condition=ready pod --all -n openshift-serverless --timeout=300s

# Create an Knative instance
oc apply -f openshift/serverless/knativeserving-kourier.yaml

# Wait for all pods in `knative-serving` to be ready
oc get pod -n knative-serving

# Install cert-manager operator
oc apply -f openshift/cert-manager/operator.yaml

# Install KServe
export KSERVE_VERSION=v0.10.1
oc apply -f "https://github.com/kserve/kserve/releases/download/${KSERVE_VERSION}/kserve.yaml"

# Patch the inferenceservice-config according to https://kserve.github.io/website/0.10/admin/serverless/kourier_networking/
oc edit configmap/inferenceservice-config --namespace kserve
# Add the flag `"disableIstioVirtualHost": true` under the ingress section
ingress : |- {
    "disableIstioVirtualHost": true
}
oc rollout restart deployment kserve-controller-manager -n kserve
oc wait --for=condition=ready pod -l control-plane=kserve-controller-manager -n kserve --timeout=300s

# Install KServe built-in servingruntimes and storagecontainers
oc apply -f "https://github.com/kserve/kserve/releases/download/${KSERVE_VERSION}/kserve-cluster-resources.yaml"
```

## Installation with Service Mesh

```bash
# Install Service Mesh operators
oc apply -f openshift/service-mesh/operators.yaml
oc wait --for=condition=ready pod --all -n openshift-operators --timeout=300s

# Create an istio instance
oc apply -f openshift/service-mesh/namespace.yaml
oc apply -f openshift/service-mesh/smcp.yaml
oc wait --for=condition=ready pod --all -n openshift-operators --timeout=300s
oc wait --for=condition=ready pod --all -n istio-system --timeout=300s

# Make sure to add your namespaces to the ServiceMeshMemberRoll
oc create ns kserve 
oc create ns kserve-demo
oc apply -f openshift/service-mesh/smmr.yaml

# Install OpenShift Serverless operator
oc apply -f openshift/serverless/operator.yaml
oc wait --for=condition=ready pod --all -n openshift-serverless --timeout=300s

# Create a PeerAuthentication Policy for each of your namespaces
oc apply -f openshift/service-mesh/peer-authentication.yaml

# Create an Knative instance
oc apply -f openshift/serverless/knativeserving-istio.yaml

# Wait for all pods in `knative-serving` to be ready
oc get pod -n knative-serving

# Create the Knative gateways
# This contains a self-signed TLS certificate, you can change this to your own
# Please consider https://access.redhat.com/documentation/en-us/openshift_container_platform/4.12/html/serverless/integrations#serverlesss-ossm-external-certs_serverless-ossm-setup for more information
oc apply -f openshift/serverless/gateways.yaml

# Install cert-manager operator
oc apply -f openshift/cert-manager/operator.yaml

# Install KServe
export KSERVE_VERSION=v0.10.1
oc apply -f "https://github.com/kserve/kserve/releases/download/${KSERVE_VERSION}/kserve.yaml"

# Install KServe built-in serving runtimes and storagecontainers
oc wait --for=condition=ready pod -l control-plane=kserve-controller-manager -n kserve --timeout=300s
oc apply -f "https://github.com/kserve/kserve/releases/download/${KSERVE_VERSION}/kserve-cluster-resources.yaml"

# Add NetworkPolicies to allow traffic to kserve webhook
oc apply -f openshift/networkpolicies.yaml
```

# Test KServe Installation

## Prerequisites
```bash
# Create a namespace for testing
oc create ns kserve-demo

# Allow pods to run as user `knative/1000` for the KServe python images, see python/*.Dockerfile
oc adm policy add-scc-to-user anyuid -z default -n kserve-demo
```

## Routing

OpenShift Serverless integrates with the OpenShift Ingress controller. So in contrast to KServe on Kubernetes, in OpenShift you automatically get routable domains for each KServe service. 

## Testing

### With Kourier

Create an inference service. From the docs of the `kserve` repository, run:

```bash
oc apply -f ./samples/v1beta1/sklearn/v1/sklearn.yaml -n kserve-demo
```

Give it a minute, then check the InferenceService status:

```bash
oc get inferenceservices sklearn-iris -n kserve-demo

NAME           URL                                                                                                            READY   PREV   LATEST   PREVROLLEDOUTREVISION   LATESTREADYREVISION                    AGE
sklearn-iris   http://sklearn-iris-predictor-default-kserve-demo.apps.<your-cluster-domain>   True           100                              sklearn-iris-predictor-default-00001   44s
```

**Note:** It is possible that the `InferenceService` shows `http` as protocol. Depending on your OpenShift installation, this usually is `https`, so try calling the service with `https` in that case.

Now you try curling the InferenceService using the routable URL from above:
```bash
curl -k https://sklearn-iris-predictor-default-kserve-demo.apps.<your-cluster-domain>/v1/models/sklearn-iris:predict -d '{"instances": [[6.8,  2.8,  4.8,  1.4],[6.0,  3.4,  4.5,  1.6]]}'
```

You should see an output like:

```
{"predictions": [1, 1]}
```

You can now try more examples from https://kserve.github.io/website/.


### With Service Mesh (Istio)

Service Mesh in OpenShift Container Platform requires some annotations to be present on a `KnativeService`. Those annotations can be propagated from the `InferenceService` and `InferenceGraph`. For this, you need to add the following annotations to your resources:

> 📝 Note: OpenShift runs istio with istio-cni enabled. To allow init-containers to call out to DNS and other external services like S3 buckets, the KServes storage-initializer init-container must run as the same user id as the istio-proxy.
> In OpenShift, the istio-proxy gets the user-id of the namespace incremented by 1 assigned. You have to specify the annotation `serving.kserve.io/storage-initializer-uid` with the same value.
> You can get your annotation range from your namespace using:
> 
> ```bash
> oc describe namespace <your-namespace>
> ```
> and check for `openshift.io/sa.scc.uid-range=1008050000/10000`
> 
> More details on the root cause can be found here: https://istio.io/latest/docs/setup/additional-setup/cni/#compatibility-with-application-init-containers.

```yaml
apiVersion: "serving.kserve.io/v1beta1"
kind: "InferenceService"
metadata:
  ...
  annotations:
    sidecar.istio.io/inject: "true"
    sidecar.istio.io/rewriteAppHTTPProbers: "true"
    serving.knative.openshift.io/enablePassthrough: "true"
    serving.kserve.io/storage-initializer-uid: "1008050001" # has to be changed to your namespaces value, see note above
spec:
...
```
```yaml
apiVersion: "serving.kserve.io/v1alpha1"
kind: "InferenceGraph"
metadata:
  ...
  annotations:
    sidecar.istio.io/inject: "true"
    sidecar.istio.io/rewriteAppHTTPProbers: "true"
    serving.knative.openshift.io/enablePassthrough: "true"
    serving.kserve.io/storage-initializer-uid: "1008050001" # has to be changed to your namespaces value, see note above
spec:
...
```

So you can do the same example as above, including the annotations:

```bash
cat <<EOF | oc apply -f -
apiVersion: "serving.kserve.io/v1beta1"
kind: "InferenceService"
metadata:
  name: "sklearn-iris"
  namespace: kserve-demo
  annotations:
    sidecar.istio.io/inject: "true"
    sidecar.istio.io/rewriteAppHTTPProbers: "true"
    serving.knative.openshift.io/enablePassthrough: "true"
spec:
  predictor:
    sklearn:
      storageUri: "gs://kfserving-examples/models/sklearn/1.0/model"
EOF
```

Give it a minute, then check and call it the same was as described above in the Kourier example.

