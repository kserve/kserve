# KServe on OpenShift Container Platform

[OpenShift Container Platform](https://www.openshift.com/products/container-platform) is built on top of Kubernetes,  
and offers a consistent hybrid cloud foundation for building and scaling containerized applications.

# Contents

1. [Installation](#installation)
2. [Test KServe installation](#test-kserve-installation)

# Installation

**Note**: These instructions were tested on OpenShift 4.17, with KServe 0.14.

KServe can use Knative as a model serving layer. In OpenShift Knative comes bundled as a product called  
[OpenShift Serverless](https://docs.openshift.com/container-platform/latest/serverless/about/about-serverless.html). OpenShift Serverless can be installed with two different ingress layers:

* [OpenShift Service Mesh (Istio)](https://www.redhat.com/en/technologies/cloud-computing/openshift/what-is-openshift-service-mesh)
* [Kourier](https://github.com/knative-sandbox/net-kourier)

While `Istio` is the most compatible with KServe, other Ingress classes are supported by Knative like Kourier.
Follow the next steps to configure Serverless plus the desired Ingress layer.


## Installing OpenShift Serverless

The following steps will install OpenShift Serverless Operator

The OpenShift Serverless version used at the time being is `v1.34.1`

```bash
# Install OpenShift Serverless operator
oc apply -f openshift/serverless/operator.yaml
# This might take a moment until the pods appear in the following command
oc wait --for=condition=ready pod --all -n openshift-serverless --timeout=300s
pod/knative-openshift-5f598cf56b-hk2kb condition met                                                                                                                                                           ─╯
pod/knative-openshift-ingress-6546869495-tfg2b condition met
pod/knative-operator-webhook-86c5f88574-8vckn condition met
```

These three pods must be ready and running. If for some reason the pods dont get ready, investigating the logs  
is a good start point.


The cert-manager operator is required for OpenShift Serverless. If you have not installed it yet, you can do so with:

```bash
# Install cert-manager operator
oc apply -f openshift/cert-manager/operator.yaml
oc wait --for=condition=ready pod --all -n cert-manager-operator --timeout=300s
# This might take a moment until the pods appear in the following command
pod/cert-manager-operator-controller-manager-545ccc5977-lrcpz condition met
```


> 📝 Note: If you are Running OpenShift on AWS (ROSA), you will need to prepare your AWS account for the dynamic certificates.
> Follow the steps described on [this](https://cloud.redhat.com/experts/rosa/dynamic-certificates/#prepare-aws-account) document
> Or, you can also use the community Cert Manager Operator.


Next, follow the corresponding installation guide for the `Ingress` layer you want to use:

1. [Service Mesh](#installation-with-service-mesh)
2. [Kourier](#installation-with-kourier)

## Installation with Service Mesh

For more information about OpenShift ServiceMesh and its components like Jaeger and Kiali, please visit the [docs](https://docs.openshift.com/container-platform/4.17/service_mesh/v2x/installing-ossm.html).
This is intended to be a very simple example and, we will install the Operator in the worker nodes, if you want to assign  
the ServiceMesh to the `infrastructure` nodes, please see the documentation mentioned above. Also, only the ServiceMesh
operator will be installed for simplicity and less resource consumption.

Before continuing, make sure you have followed the steps in the [Installing OpenShift Serverless](#installing-openshift-serverless) section.

```bash
# Install Service Mesh operator
oc apply -f openshift/service-mesh/operators.yaml
oc wait --for=condition=ready pod --all -n openshift-operators --timeout=300s


# Create an istio instance
oc create ns istio-system
oc apply -f openshift/service-mesh/smcp.yaml
oc wait --for=condition=ready pod --all -n openshift-operators --timeout=300s
oc wait --for=condition=ready pod --all -n istio-system --timeout=300s

# Make sure to add your namespaces to the ServiceMeshMemberRoll and create a
# PeerAuthentication Policy for each of your namespaces
oc create ns kserve; oc create ns kserve-demo
oc apply -f openshift/service-mesh/smmr.yaml
oc apply -f openshift/service-mesh/peer-authentication.yaml

# Create an Knative instance
oc apply -f openshift/serverless/knativeserving-istio.yaml

# Wait for all pods in `knative-serving` to be ready
oc wait --for=condition=ready pod --all -n knative-serving --timeout=300s
# the pods needs to be like:
# oc get pods -n knative-serving
#NAME                              READY   STATUS    RESTARTS   AGE
#activator-748f464bd6-j6ptx        2/2     Running   0          165m
#activator-748f464bd6-n9w8g        2/2     Running   0          166m
#autoscaler-6fc46ddb97-h5x7n       2/2     Running   0          166m
#autoscaler-6fc46ddb97-l28m9       2/2     Running   0          166m
#autoscaler-hpa-67cb476ddc-h6m7g   2/2     Running   0          166m
#autoscaler-hpa-67cb476ddc-rr4sg   2/2     Running   0          166m
#controller-5bb67c45f7-5tz2h       2/2     Running   0          165m
#controller-5bb67c45f7-mv68q       2/2     Running   0          166m
#webhook-6cb5d8875d-964sc          2/2     Running   0          166m
#webhook-6cb5d8875d-pg5gq          2/2     Running   0          165m

# Create the Knative gateways
# This contains a self-signed TLS certificate, you can change this to your own
# Please consider https://access.redhat.com/documentation/en-us/openshift_container_platform/4.12/html/serverless/integrations#serverlesss-ossm-external-certs_serverless-ossm-setup for more information
oc apply -f openshift/serverless/gateways.yaml

# Install KServe
KSERVE_VERSION=v0.14.1
oc apply --server-side -f "https://github.com/kserve/kserve/releases/download/${KSERVE_VERSION}/kserve.yaml"
oc wait --for=condition=ready pod -l control-plane=kserve-controller-manager -n kserve --timeout=300s

# Add NetworkPolicies to allow traffic to kserve webhook
oc apply -f openshift/networkpolicies.yaml

# Install KServe built-in serving runtimes and storagecontainers
oc apply -f "https://github.com/kserve/kserve/releases/download/${KSERVE_VERSION}/kserve-cluster-resources.yaml"
```

Also, it will be required to disable the Istio Virtual Host so it can use Knative exposed route to do the inference:
```bash
oc edit configmap/inferenceservice-config --namespace kserve
# edit this section
ingress : |- {
    "disableIstioVirtualHost": true
}
```
You are now ready to [testing](#test-kserve-installation) the installation.

## Clean-up
After [testing](#test-kserve-installation) clean-up the environment:
```bash
# Uninstall KServe
oc delete -f "https://github.com/kserve/kserve/releases/download/${KSERVE_VERSION}/kserve.yaml"
oc delete ns kserve-demo

# Uninstall Serverless operator
oc delete KnativeServing knative-serving  -n knative-serving
oc delete all --all -n openshift-serverless --force --grace-period=0 
oc patch knativeservings.operator.knative.dev knative-serving -n knative-serving -p '{"metadata": {"finalizers": []}}' --type=merge
oc delete KnativeServing --all -n knative-serving --force --grace-period=0
oc delete pod,deployment,all --all -n knative-serving  --force --grace-period=0
oc patch ns knative-serving -p '{"metadata": {"finalizers": []}}' --type=merge
oc patch ns knative-eventing -p '{"metadata": {"finalizers": []}}' --type=merge
oc patch ns rhoai-serverless -p '{"metadata": {"finalizers": []}}' --type=merge
oc patch ns openshift-serverless -p '{"metadata": {"finalizers": []}}' --type=merge

oc delete validatingwebhookconfiguration $(oc get validatingwebhookconfiguration --no-headers|grep -E "kserve|knative|istio"|awk '{print $1}')
oc delete mutatingwebhookconfiguration $(oc get mutatingwebhookconfiguration --no-headers|grep -E "kserve|knative|istio"|awk '{print $1}')

oc delete ServiceMeshControlPlane,pod --all -n istio-system --force --grace-period=0  -n istio-system
oc patch smcp $(oc get smcp -n istio-system | awk 'NR>1 {print $1}') -n istio-system -p '{"metadata": {"finalizers": []}}' --type=merge
oc delete smcp -n istio-system $(oc get smcp -n istio-system | awk 'NR>1 {print $1}')
oc patch smmr $(oc get smmr -n istio-system | awk 'NR>1 {print $1}') -n istio-system -p '{"metadata": {"finalizers": []}}' --type=merge
oc delete smm --all --force --grace-period=0  -A
oc delete smmr,smm --all --force --grace-period=0  -n istio-system
oc patch ns istio-system -p '{"metadata": {"finalizers": []}}' --type=merge
oc delete ns istio-system

oc delete subscription serverless-operator -n openshift-serverless
oc delete csv $(oc get csv -n openshift-serverless -o jsonpath='{.items[?(@.spec.displayName=="Red Hat OpenShift Serverless")].metadata.name}') -n openshift-serverless
oc delete csv OperatorGroup serverless-operators -n openshift-serverless

oc delete sub servicemeshoperator --force --grace-period=0 -n openshift-operators
oc delete svc maistra-admission-controller -n openshift-operators
oc delete csv $(oc get csv -n openshift-operators | grep servicemeshoperator|awk '{print $1}') -n openshift-operators

# Uninstall cert-manager operator
oc delete -f openshift/cert-manager/operator.yaml
```

## Installation with Kourier

Kourier is a lightweight Kubernetes Ingress controller specifically designed for Knative, which is a serverless platform built on Kubernetes. In the context of KServe:
Key points:

- Handles network routing for serverless model inference deployments
- Provides efficient traffic management
- Enables external access to KServe model serving endpoints
- Supports features like traffic splitting and canary deployments

Let's naviagate through the installation steps:

The Knative Kourier plugin version used at the time being is `knative-v1.17.0`

Before continuing, make sure you have followed the steps in the [Installing OpenShift Serverless](#installing-openshift-serverless) section.

```bash
# install the knative-kourier
oc apply -f openshift/serverless/knativeserving-kourier.yaml
```

If you encounter this error: 
```asciidoc
Error creating: pods "3scale-kourier-gateway-bc6d7f8cf-" is forbidden: unable to validate against any  
security context constraint: [provider "anyuid": Forbidden: not usable by user or serviceaccount, provider  
restricted-v2: .containers[0].runAsUser: Invalid value: 65534: must be in the ranges: [1000750000, 1000759999],
```
It can be patched by:
```bash
oc patch deployment 3scale-kourier-gateway -n kourier-system --type=json \
  -p='[{"op": "remove", "path": "/spec/template/spec/containers/0/securityContext/runAsUser"}, {"op": "remove", "path": "/spec/template/spec/containers/0/securityContext/runAsGroup"}]'
```

Execute this patch to knative-serving can be exposed through Kourier:

```bash
# wait for all the pods to get ready
oc get pods -n knative-serving && kubectl get pods -n kourier-system

NAME                                                            READY   STATUS      RESTARTS   AGE
activator-748f464bd6-gvvpr                                      2/2     Running     0          2m41s
activator-748f464bd6-mmb6d                                      2/2     Running     0          2m56s
autoscaler-6fc46ddb97-9q8vn                                     2/2     Running     0          2m55s
autoscaler-6fc46ddb97-lrj4v                                     2/2     Running     0          2m55s
autoscaler-hpa-67cb476ddc-c456b                                 2/2     Running     0          2m54s
autoscaler-hpa-67cb476ddc-vxncw                                 2/2     Running     0          2m54s
controller-84554d6cc8-nk7b7                                     2/2     Running     0          2m19s
controller-84554d6cc8-pllk6                                     2/2     Running     0          2m54s
net-kourier-controller-69db6f665f-4bnvx                         1/1     Running     0          16s
storage-version-migration-serving-serving-latest-1.34.1-hvxz4   0/1     Completed   0          2m54s
webhook-6cb5d8875d-4hbs2                                        2/2     Running     0          2m40s
webhook-6cb5d8875d-srk4z                                        2/2     Running     0          2m55s
NAME                                      READY   STATUS    RESTARTS   AGE
3scale-kourier-gateway-56b445cd8c-69cq7   1/1     Running   0          16s

# then, patch the knative service ingress class
oc patch configmap/config-network \
  --namespace knative-serving \
  --type merge \
  --patch '{"data":{"ingress.class":"kourier.ingress.networking.knative.dev"}}'
```

Once all are ready, let's install KServe
```bash
# Install KServe
KSERVE_VERSION=v0.14.1
oc apply --server-side -f "https://github.com/kserve/kserve/releases/download/${KSERVE_VERSION}/kserve.yaml"

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

## Clean-up
After [testing](#test-kserve-installation) clean-up the environment:
```bash
# Uninstall KServe
oc delete -f "https://github.com/kserve/kserve/releases/download/${KSERVE_VERSION}/kserve.yaml"
oc delete -f "https://github.com/kserve/kserve/releases/download/${KSERVE_VERSION}/kserve-cluster-resources.yaml"

# Uninstall Kourier
oc delete -f openshift/serverless/knativeserving-kourier.yaml

oc delete ns kserve-demo

# Uninstall Serverless operator
oc delete KnativeServing knative-serving  -n knative-serving
oc delete all --all -n openshift-serverless --force --grace-period=0 
oc patch knativeservings.operator.knative.dev knative-serving -n knative-serving -p '{"metadata": {"finalizers": []}}' --type=merge
oc delete KnativeServing --all -n knative-serving --force --grace-period=0
oc delete pod,deployment,all --all -n knative-serving  --force --grace-period=0
oc patch ns knative-serving -p '{"metadata": {"finalizers": []}}' --type=merge
oc patch ns knative-eventing -p '{"metadata": {"finalizers": []}}' --type=merge
oc patch ns rhoai-serverless -p '{"metadata": {"finalizers": []}}' --type=merge
oc patch ns openshift-serverless -p '{"metadata": {"finalizers": []}}' --type=merge

oc delete validatingwebhookconfiguration $(oc get validatingwebhookconfiguration --no-headers|grep -E "kserve|knative|istio"|awk '{print $1}')
oc delete mutatingwebhookconfiguration $(oc get mutatingwebhookconfiguration --no-headers|grep -E "kserve|knative|istio"|awk '{print $1}')

oc delete subscription serverless-operator -n openshift-serverless
oc delete csv $(oc get csv -n openshift-serverless -o jsonpath='{.items[?(@.spec.displayName=="Red Hat OpenShift Serverless")].metadata.name}') -n openshift-serverless
oc delete csv OperatorGroup serverless-operators -n openshift-serverless

# Uninstall cert-manager operator
oc delete -f openshift/cert-manager/operator.yaml
```
You are now ready to [testing](#test-kserve-installation) the installation.


# Test KServe Installation

If you hit any `SecurityContext` issues related with the `knative/1000` user, you can allow the pods to run as this user:
```bash
# Create a namespace for testing
oc create ns kserve-demo

# Allow pods to run as user `knative/1000` for the KServe python images, see python/*.Dockerfile
oc adm policy add-scc-to-user anyuid -z default -n kserve-demo
```

## Routing

OpenShift Serverless integrates with the OpenShift Ingress controller. So in contrast to KServe on Kubernetes,  
in OpenShift you automatically get routable domains for each KServe service. 

## Testing

### With Kourier

Create an inference service. From the docs of the `kserve` repository, run:

```bash
oc apply -f ./samples/v1beta1/sklearn/v1/sklearn.yaml -n kserve-demo
```

Give it a minute, then check the InferenceService status:

```bash
oc get inferenceservices sklearn-iris -n kserve-demo

NAME           URL                                                                            READY   PREV   LATEST   PREVROLLEDOUTREVISION   LATESTREADYREVISION                    AGE
sklearn-iris   http://sklearn-iris-predictor-default-kserve-demo.apps.<your-cluster-domain>   True           100                              sklearn-iris-predictor-default-00001   44s
```

**Note:** It is possible that the `InferenceService` shows `http` as protocol. Depending on your OpenShift  
installation, this usually is `https`, so try calling the service with `https` in that case.

Now you try curling the InferenceService using the routable URL from above:
```bash
MODEL_NAME=sklearn-iris
INPUT_PATH=@samples/v1beta1/sklearn/v1/iris-input.json
SERVICE_HOSTNAME=$(oc get inferenceservice sklearn-iris -o jsonpath='{.status.url}' -n kserve-demo | cut -d "/" -f 3)
echo "Using payload from $INPUT_PATH"
cat $(echo $INPUT_PATH | cut -d@ -f 2)
curl -k -H "Host: ${SERVICE_HOSTNAME}" -H "Content-Type: application/json" \
  https://$SERVICE_HOSTNAME/v1/models/$MODEL_NAME:predict -d $INPUT_PATH
```

You should see an output like:

```
{"predictions":[1,1]}
```

You can now try more examples from https://kserve.github.io/website/.


### With Service Mesh (Istio)

Service Mesh in OpenShift Container Platform requires some annotations to be present on a `KnativeService`.  
Those annotations can be propagated from the `InferenceService` and `InferenceGraph`. For this, you need to add  
the following annotations to your resources:


> 📝 Note: OpenShift runs istio with istio-cni enabled. To allow init-containers to call out to DNS and other external  
> services like S3 buckets, the KServes storage-initializer init-container must run as the same user id as the istio-proxy.
> In OpenShift, the istio-proxy gets the user-id of the namespace incremented by 1 assigned. You have to specify the  
> annotation `serving.kserve.io/storage-initializer-uid` with the same value.
> You can get your annotation range from your namespace using:
> 
> ```bash
> oc describe namespace kserve-demo
> ```
> and check for `openshift.io/sa.scc.uid-range=1000860000/10000`
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
    serving.kserve.io/storage-initializer-uid: "1000860001" # has to be changed to your namespaces value, see note above
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
    serving.kserve.io/storage-initializer-uid: "1000860001" # has to be changed to your namespaces value, see note above
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
    serving.kserve.io/storage-initializer-uid: "1000860001"
spec:
  predictor:
    sklearn:
      storageUri: "gs://kfserving-examples/models/sklearn/1.0/model"
EOF
```

Once the `InferenceService` is ready, you can test it with the same curl command as mentioned [here](#test-kserve-installation).
