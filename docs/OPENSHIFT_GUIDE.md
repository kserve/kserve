# KFServing on OpenShift

[OpenShift Container Platform](https://www.openshift.com/products/container-platform) is built on top of Kubernetes, and offers a consistent hybrid cloud foundation for building and scaling containerized applications.

# Contents

1. [Installation](#installation)
2. [Test KFServing installation](#test-kfserving-installation)
3. [(Optional) Change domain used for InferenceServices](#change-knative-domain-configuration)

# Installation

Installation of standalone KFServing on OpenShift can be done in multiple ways depending on your environment. The easiest way is to use the [`quick_install.sh`](../hack/quick_install.sh) script provided in this repository. This assumes you do not already have Istio and Knative running on your cluster. However, if you are currently using OpenShift Service Mesh, please go to the corresponding installation guide below.

1. [Option 1: Quick install](#quick-install)
2. [Option 2: Install using OpenShift Service Mesh](#install-using-openshift-service-mesh)

## Quick install

**Note**: These instructions were tested on OpenShift 4.5.24, with KFServing v0.5.0-rc2, Istio 1.6.2, and Knative 0.18.0
which are in the quick install script. Additionally, we have tested it with Kubeflow 1.2 recommended versions
for Istio and Knative, i.e. Istio 1.3.1 and Knative 0.14.3.

### 1. Clone repository

```bash
git clone https://github.com/kubeflow/kfserving
```

### 2. Add Security Context Constraint (SCC)

Run the following to enable containers to run with UID 0 for Istio’s service accounts, as recommended on [Istio's installation instructions for OpenShift](https://istio.io/latest/docs/setup/platform-setup/openshift/).

```bash
oc adm policy add-scc-to-group anyuid system:serviceaccounts:istio-system
```

### 3. Run install script

From the root of the `kfserving` directory, execute the following:

```bash
# Ensure we install KFServing v0.4.1
sed -i.bak 's/KFSERVING_VERSION=.*/KFSERVING_VERSION=v0.5.0-rc2/' ./hack/quick_install.sh
./hack/quick_install.sh
```

This [script](../hack/quick_install.sh) will install Istio, Knative, Cert Manager, and then the latest version of KFServing that has been verified and tested on OpenShift.

### 4. Verify KFServing installation

Check that the KFserving controller is running:

```bash
oc get po -n kfserving-system

NAME                             READY   STATUS    RESTARTS   AGE
kfserving-controller-manager-0   2/2     Running   0          2m28s
```

### 5. Expose OpenShift route

After installation is verified, expose an OpenShift route for the ingress gateway.

```bash
oc -n istio-system expose svc/istio-ingressgateway --port=http2
```

At this point you can now [test the KFServing installation](#test-kfserving-installation).

## Install using OpenShift Service Mesh

These instructions were tested using [OpenShift Service Mesh 2.0](https://www.openshift.com/blog/introducing-openshift-service-mesh-2.0) on OpenShift 4.5.24.

### 1. Install OpenShift Service Mesh

If you have not already done so, install the OpenShift Service Mesh Operator and deploy the control plane. An installation guide can be found [here](https://docs.openshift.com/container-platform/4.6/service_mesh/v2x/installing-ossm.html).

### 2. Change istio-ingressgateway service type

To allow external traffic, ensure that the service type of `istio-ingressgateway` is either `NodePort` or `LoadBalancer`.

```bash
oc get svc istio-ingressgateway -n istio-system
```

If the type is ClusterIP, change it using one of the following commands.

```bash
oc patch svc istio-ingressgateway -n istio-system -p '{"spec":{"type":"NodePort"}}'
# or
oc patch svc istio-ingressgateway -n istio-system -p '{"spec":{"type":"LoadBalancer"}}'
```

### 3. Install Knative-Serving

```bash
KNATIVE_VERSION=v0.18.0

oc apply --filename https://github.com/knative/serving/releases/download/${KNATIVE_VERSION}/serving-crds.yaml
oc apply --filename https://github.com/knative/serving/releases/download/${KNATIVE_VERSION}/serving-core.yaml
oc apply --filename https://github.com/knative/net-istio/releases/download/${KNATIVE_VERSION}/release.yaml
```

### 4. Create cluster local gateway

Currently, KFServing (and Knative by default) expects the Knative local gateway to be `cluster-local-gateway`. OpenShift Service Mesh does not have any `cluster-local-gateway`
service or deployment in the `istio-system` namespace. The following creates a `cluster-local-gateway` service that can be used.

```bash
cat <<EOF | oc apply -f -
apiVersion: v1
kind: Service
metadata:
  name: cluster-local-gateway
  namespace: istio-system
  labels:
    serving.knative.dev/release: "v0.18.0"
    networking.knative.dev/ingress-provider: istio
spec:
  type: ClusterIP
  selector:
    istio: ingressgateway
  ports:
    - name: http2
      port: 80
      targetPort: 8081
---
apiVersion: networking.istio.io/v1alpha3
kind: Gateway
metadata:
  name: cluster-local-gateway
  namespace: knative-serving
  labels:
    serving.knative.dev/release: "v0.18.0"
    networking.knative.dev/ingress-provider: istio
spec:
  selector:
    istio: ingressgateway
  servers:
    - port:
        number: 8081
        name: http
        protocol: HTTP
      hosts:
        - "*"
EOF
```

### 5. Install cert-manager

```bash
kubectl apply --validate=false -f https://github.com/jetstack/cert-manager/releases/download/v0.15.1/cert-manager.yaml
kubectl wait --for=condition=available --timeout=600s deployment/cert-manager-webhook -n cert-manager
```

### 6. Install KFserving

```bash
git clone https://github.com/kubeflow/kfserving
cd kfserving
export KFSERVING_VERSION=v0.5.0-rc2
oc apply -f install/${KFSERVING_VERSION}/kfserving.yaml
```

### 7. Verify KFServing installation

Check that the KFserving controller is running:

```bash
oc get po -n kfserving-system

NAME                             READY   STATUS    RESTARTS   AGE
kfserving-controller-manager-0   2/2     Running   0          2m28s
```

### 8. Add namespaces to Service Mesh member roll

To add projects to the mesh, namespaces need to be added to the member roll. In the following command, a
`ServiceMeshMemberRoll` resource is created with the `knative-serving` and `kfserving-system` namespaces as members.
Namespaces that will contain KFServing InferenceServices also need to be added. Here, the `kfserving-test` namespace is added,
but additional namespaces can be added as well.

```bash
cat <<EOF | oc apply -f -
apiVersion: maistra.io/v1
kind: ServiceMeshMemberRoll
metadata:
  name: default
  namespace: istio-system
spec:
  members:
    - knative-serving
    - kfserving-system
    - kfserving-test
EOF
```

### 9. Add Network Policies

Create these `NetworkPolicy` resources to allow all ingress traffic to pods in the
`knative-serving` and `kfserving-system` namespaces.

```bash
cat <<EOF | oc apply -f -
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: knative-serving-pods
  namespace: knative-serving
spec:
  podSelector: {}
  ingress:
  - {}
---
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: kfserving-controller-manager
  namespace: kfserving-system
spec:
  podSelector: {}
  ingress:
  - {}
EOF
```

At this point you can now [test the KFServing installation](#test-kfserving-installation).

# Test KFServing installation

Create an inference service. From the root of the `kfserving` directory, run:

```bash
oc create ns kfserving-test
API_VERSION=v1beta1
oc apply -f docs/samples/${API_VERSION}/sklearn/sklearn.yaml -n kfserving-test
```

Give it a minute, then check the InferenceService status:

```bash
oc get inferenceservices sklearn-iris -n kfserving-test

NAME           URL                                              READY   PREV   LATEST   PREVROLLEDOUTREVISION   LATESTREADYREVISION                    AGE
sklearn-iris   http://sklearn-iris.kfserving-test.example.com   True           100                              sklearn-iris-predictor-default-z5lqk   53s                             3m37s
```

Once the InferenceService is ready, try curling it for a prediction:

```bash
export INGRESS_HOST=$(oc get route istio-ingressgateway -n istio-system -ojsonpath='{.spec.host}')
export INGRESS_PORT=$(oc -n istio-system get service istio-ingressgateway -o jsonpath='{.spec.ports[?(@.name=="http2")].nodePort}')
export SERVICE_HOSTNAME=$(oc get inferenceservice sklearn-iris -n kfserving-test -o jsonpath='{.status.url}' | cut -d "/" -f 3)
curl -v -H "Host: ${SERVICE_HOSTNAME}" http://${INGRESS_HOST}:${INGRESS_PORT}/v2/models/sklearn-iris/infer -d @./docs/samples/v1beta1/sklearn/iris-input.json
```

You should see an output like:

```
{
  "model_name":"sklearn-iris",
  "model_version":"v1",
  "id":"f7db5014-4a5a-4374-b45e-ffa2df4d5a6d",
  "parameters":null,
  "outputs":[{
      "name":"predict",
      "shape":[2],
      "datatype":"FP32",
      "parameters":null,
      "data":[1, 2]
    }
  ]
}
```

# Change Knative domain configuration

This is optional, but if you want to change the domain that is used for the InferenceService routes,
do the following:

```bash
oc edit configmap config-domain -n knative-serving
```

This will open your default text editor, and you will see something like:

```YAML
apiVersion: v1
data:
  _example: |
    ################################
    #                              #
    #    EXAMPLE CONFIGURATION     #
    #                              #
    ################################
    # ...
    example.com: |
kind: ConfigMap
...
```

Add a line above the `_example` key with your hostname as the key and an empty string value.
Be sure to update `<hostname>`:

```YAML
apiVersion: v1
data:
    <hostname>: ""
    _example: |
    ...
kind: ConfigMap
...
```

As an example, with OpenShift on IBM Cloud, the ConfigMap might look something like:

```YAML
apiVersion: v1
data:
    pv-cluster-442dbba0442be6c8c50f31ed96b00601-0000.sjc03.containers.appdomain.cloud: ""
    _example: |
    ...
kind: ConfigMap
...
```

After you save and exit, the routes for your InferenceServices will start using this new domain.
You can curl the endpoints without the `Host` header. For example:

```bash
curl -v http://sklearn-iris.kfserving-test.pv-cluster-442dbba0442be6c8c50f31ed96b00601-0000.sjc03.containers.appdomain.cloud:${INGRESS_PORT}/v2/models/sklearn-iris/infer -d @./docs/samples/v1beta1/sklearn/iris-input.json
```
