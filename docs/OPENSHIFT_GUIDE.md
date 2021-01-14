# KFServing on OpenShift

[OpenShift Container Platform](https://www.openshift.com/products/container-platform) is built on top of Kubernetes, and offers a consistent hybrid cloud foundation for building and scaling containerized applications. To install standalone KFServing on OpenShift, the easiest way is to use the [`quick_install.sh`](../hack/quick_install.sh) script provided in this repository. This assumes you do not already have Istio and Knative running on your cluster.

**Note**: These instructions were tested on OpenShift 4.5.15, with KFServing 0.4.1, Istio 1.6.2, and Knative 0.15.0 which
are in the quick install script. Additionally, we have tested it with Kubeflow 1.2 recommended versions for Istio and
Knative, i.e. Istio 1.3.1 and Knative 0.14.3.

## Clone repository

```bash
git clone https://github.com/kubeflow/kfserving
```

## Add Security Context Constraint (SCC)

Run the following to enable containers to run with UID 0 for Istio’s service accounts, as recommended on [Istio's installation instructions for OpenShift](https://istio.io/latest/docs/setup/platform-setup/openshift/)

```bash
oc adm policy add-scc-to-group anyuid system:serviceaccounts:istio-system
```

## Run install script

From the root of the `kfserving` directory, execute the following:

```bash
# Ensure we install KFServing v0.4.1
sed -i.bak 's/KFSERVING_VERSION=.*/KFSERVING_VERSION=v0.4.1/' ./hack/quick_install.sh
./hack/quick_install.sh
```

This [script](../hack/quick_install.sh) will install Istio, Knative, Cert Manager, and then finally the latest version of KFServing that has been verified on OpenShift.

## Verify KFServing installation

Check that the KFserving controller is running:

```bash
oc get po -n kfserving-system

NAME                             READY   STATUS    RESTARTS   AGE
kfserving-controller-manager-0   2/2     Running   0          2m28s
```

## Expose OpenShift route

After installation is verified, expose an OpenShift route for the ingress gateway.

```bash
oc -n istio-system expose svc/istio-ingressgateway --port=http2
```

## Test KFServing installation

Now, create an inference service. From the root of the `kfserving` directory, run:

```bash
oc create ns kfserving-test
API_VERSION=v1alpha2
oc apply -f docs/samples/${API_VERSION}/sklearn/sklearn.yaml -n kfserving-test
```

Give it a minute, then check the InferenceService status:

```bash
oc get inferenceservices sklearn-iris -n kfserving-test

NAME           URL                                              READY   DEFAULT TRAFFIC   CANARY TRAFFIC   AGE
sklearn-iris   http://sklearn-iris.kfserving-test.example.com   True    100                                3m37s
```

Once the InferenceService is ready, try curling it for a prediction:

```bash
export INGRESS_HOST=$(oc get route istio-ingressgateway -n istio-system -ojsonpath='{.spec.host}')
export INGRESS_PORT=$(oc -n istio-system get service istio-ingressgateway -o jsonpath='{.spec.ports[?(@.name=="http2")].nodePort}')
export SERVICE_HOSTNAME=$(oc get inferenceservice sklearn-iris -n kfserving-test -o jsonpath='{.status.url}' | cut -d "/" -f 3)
curl -v -H "Host: ${SERVICE_HOSTNAME}" http://${INGRESS_HOST}:${INGRESS_PORT}/v1/models/sklearn-iris:predict -d @./docs/samples/sklearn/iris-input.json
```

You should see a prediction output like:

```bash
{"predictions": [1, 1]}
```
