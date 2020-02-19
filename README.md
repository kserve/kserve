# KFServing
KFServing provides a Kubernetes [Custom Resource Definition](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/) for serving machine learning (ML) models on arbitrary frameworks. It aims to solve production model serving use cases by providing performant, high abstraction interfaces for common ML frameworks like Tensorflow, XGBoost, ScikitLearn, PyTorch, and ONNX.

It encapsulates the complexity of autoscaling, networking, health checking, and server configuration to bring cutting edge serving features like GPU Autoscaling, Scale to Zero, and Canary Rollouts to your ML deployments. It enables a simple, pluggable, and complete story for Production ML Serving including prediction, pre-processing, post-processing and explainability.

![KFServing](/docs/diagrams/kfserving.png)

### Learn More
To learn more about KFServing, how to deploy it as part of Kubeflow, how to use various supported features, and how to participate in the KFServing community, please follow the [KFServing docs on the Kubeflow Website](https://www.kubeflow.org/docs/components/serving/kfserving/).

### Prerequisites
Knative Serving and Istio should be available on Kubernetes Cluster, Knative depends on Istio Ingress Gateway to route requests to Knative services.
- [Istio](https://knative.dev/docs/install/installing-istio): v1.1.7+

If you want to get up running Knative quickly or you do not need service mesh, we recommend installing Istio without service mesh(sidecar injection).
- [Knative Serving](https://knative.dev/docs/install/knative-with-any-k8s): v0.9.x+

Currently only `Knative Serving` is required, `cluster-local-gateway` is required to serve cluster-internal traffic for transformer and explainer use cases. Please follow instructions here to install [cluster local gateway](https://knative.dev/docs/install/installing-istio/#updating-your-install-to-use-cluster-local-gateway)

- [Cert Manager](https://cert-manager.io/docs/installation/kubernetes): v1.12.0+

Cert manager is needed to provision KFServing webhook certs for production grade installation, alternatively you can run our self signed certs
generation [script](./hack/self-signed-ca.sh).

### Install KFServing
```
TAG=0.2.2
kubectl apply -f ./install/$TAG/kfserving.yaml
```
By default, you can create InferenceService instances in any namespace which has no label with `control-plane` as key.
You can also configure KFServing to make InferenceService instances only work in the namespace which has label pair `serving.kubeflow.org/inferenceservice: enabled`. To enable this mode, you need to add `env` field as stated below to `manager` container of statefulset `kfserving-controller-manager`.

```
env:
- name: ENABLE_WEBHOOK_NAMESPACE_SELECTOR
  value: enabled
```
### Install KFServing in 5 Minutes (On your local machine)

Make sure you have
[kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/#install-kubectl-on-linux),
[kustomize](https://github.com/kubernetes-sigs/kustomize/blob/master/docs/INSTALL.md) v3.5.4+,
[helm 3](https://helm.sh/docs/intro/install) installed before you start.(2 mins for setup)
1) If you do not have an existing kubernetes cluster you can create a quick kubernetes local cluster with [kind](https://github.com/kubernetes-sigs/kind#installation-and-usage).(this takes 30s)
```bash
kind create cluster
```
2) Install Istio lean version, Knative Serving, KFServing all in one.(this takes 30s)
```bash
./hack/quick_install.sh
```
### Test KFServing Installation 

1) To check if KFServing Controller is installed correctly, please run the following command
```shell
kubectl get po -n kfserving-system
NAME                             READY   STATUS    RESTARTS   AGE
kfserving-controller-manager-0   2/2     Running   2          13m
```

Please refer to our [troubleshooting section](docs/DEVELOPER_GUIDE.md#troubleshooting) for recommendations and tips for issues with installation.

2) Wait all pods to be ready then launch KFServing `InferenceService`.(wait 1 min for everything to be ready and 40s to
launch the `InferenceService`)
```bash
kubectl apply -f docs/samples/sklearn/sklearn.yaml
```
3) Check KFServing `InferenceService` status.
```bash
kubectl get inferenceservices sklearn-iris
NAME           URL                                                              READY   DEFAULT TRAFFIC   CANARY TRAFFIC   AGE
sklearn-iris   http://sklearn-iris.default.example.com/v1/models/sklearn-iris   True    100                                109s
```
4) Curl the `InferenceService`
```bash
kubectl port-forward --namespace istio-system $(kubectl get pod --namespace istio-system --selector="app=istio-ingressgateway" --output jsonpath='{.items[0].metadata.name}') 8080:80
curl -v -H "Host: sklearn-iris.default.example.com" http://localhost:8080/v1/models/sklearn-iris:predict -d @./docs/samples/sklearn/iris-input.json
```
### Use KFServing SDK
* Install the SDK
  ```
  pip install kfserving
  ```
* Get the KFServing SDK documents from [here](python/kfserving/README.md).

* Follow the [example here](docs/samples/client/kfserving_sdk_sample.ipynb) to use the KFServing SDK to create, rollout, promote, and delete an InferenceService instance.

### KFServing Examples 
[KFServing examples](./docs/samples/README.md)

### KFServing Concepts and Data Plane
[KFServing Concepts and Data Plane](./docs/README.md)

### KFServing API Reference
[KFServing API Docs](./docs/apis/README.md)

### Developer Guide
[Developer Guide](/docs/DEVELOPER_GUIDE.md).

### Contributor Guide
[Contributor Guide](./CONTRIBUTING.md)
