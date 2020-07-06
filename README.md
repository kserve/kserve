# KFServing
KFServing provides a Kubernetes [Custom Resource Definition](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/) for serving machine learning (ML) models on arbitrary frameworks. It aims to solve production model serving use cases by providing performant, high abstraction interfaces for common ML frameworks like Tensorflow, XGBoost, ScikitLearn, PyTorch, and ONNX.

It encapsulates the complexity of autoscaling, networking, health checking, and server configuration to bring cutting edge serving features like GPU Autoscaling, Scale to Zero, and Canary Rollouts to your ML deployments. It enables a simple, pluggable, and complete story for Production ML Serving including prediction, pre-processing, post-processing and explainability.

![KFServing](/docs/diagrams/kfserving.png)

### Learn More
To learn more about KFServing, how to deploy it as part of Kubeflow, how to use various supported features, and how to participate in the KFServing community, please follow the [KFServing docs on the Kubeflow Website](https://www.kubeflow.org/docs/components/serving/kfserving/).

### Prerequisites
Knative Serving and Istio should be available on Kubernetes Cluster, Knative depends on Istio Ingress Gateway to route requests to Knative services. To use the exact versions tested by the Kubeflow and KFServing teams, please refer to the [prerequisites on developer guide](docs/DEVELOPER_GUIDE.md#install-knative-on-a-kubernetes-cluster)

- [Istio](https://knative.dev/docs/install/installing-istio): v1.1.6+

If you want to get up running Knative quickly or you do not need service mesh, we recommend installing Istio without service mesh(sidecar injection).
- [Knative Serving](https://knative.dev/docs/install/knative-with-any-k8s): v0.11.2+

Currently only `Knative Serving` is required, `cluster-local-gateway` is required to serve cluster-internal traffic for transformer and explainer use cases. Please follow instructions here to install [cluster local gateway](https://knative.dev/docs/install/installing-istio/#updating-your-install-to-use-cluster-local-gateway)

- [Cert Manager](https://cert-manager.io/docs/installation/kubernetes): v0.12.0+

Cert manager is needed to provision KFServing webhook certs for production grade installation, alternatively you can run our self signed certs
generation [script](./hack/self-signed-ca.sh).

### Install KFServing
#### Standalone KFServing Installation
KFServing can be installed standalone if your kubernetes cluster meets the above prerequisites and KFServing controller is deployed in `kfserving-system` namespace.
```
TAG=v0.3.0
kubectl apply -f ./install/$TAG/kfserving.yaml
```
KFServing uses pod mutator or [mutating admission webhooks](https://kubernetes.io/blog/2019/03/21/a-guide-to-kubernetes-admission-controllers/) to inject the storage initializer component of KFServing. By default all the pods in namespaces which are not labelled with `control-plane` label go through the pod mutator.
This can cause problems and interfere with Kubernetes control panel when KFServing pod mutator webhook is not in ready state yet.

For Kubernetes 1.14 users we suggest enabling the following environment variable `ENABLE_WEBHOOK_NAMESPACE_SELECTOR` so that only pods
 in the namespaces which are labelled `serving.kubeflow.org/inferenceservice: enabled` go through the KFServing pod mutator.
```
env:
- name: ENABLE_WEBHOOK_NAMESPACE_SELECTOR
  value: enabled
```

For Kubernetes 1.15+ users we strongly suggest turning on the [object selector](https://kubernetes.io/docs/reference/access-authn-authz/extensible-admission-controllers/#matching-requests-objectselector) so that only KFServing `InferenceService` pods go through the pod mutator.
```bash
kubectl patch mutatingwebhookconfiguration inferenceservice.serving.kubeflow.org --patch '{"webhooks":[{"name": "inferenceservice.kfserving-webhook-server.pod-mutator","objectSelector":{"matchExpressions":[{"key":"serving.kubeflow.org/inferenceservice", "operator": "Exists"}]}}]}'
```
#### KFServing in Kubeflow Installation
KFServing is installed by default as part of Kubeflow installation using [Kubeflow manifests](https://github.com/kubeflow/manifests/tree/master/kfserving) and KFServing controller is deployed in `kubeflow` namespace.
Since Kubeflow Kubernetes minimal requirement is 1.14 which does not support object selector, `ENABLE_WEBHOOK_NAMESPACE_SELECTOR` is enabled in Kubeflow installation by default.
If you are using Kubeflow dashboard or [profile controller](https://www.kubeflow.org/docs/components/multi-tenancy/getting-started/#manual-profile-creation) to create  user namespaces, labels are automatically added to enable KFServing to deploy models. If you are creating namespaces manually using Kubernetes apis directly, you will need to add label `serving.kubeflow.org/inferenceservice: enabled` to allow deploying KFServing `InferenceService` in the given namespaces, and do ensure you do not deploy
`InferenceService` in `kubeflow` namespace which is labelled as `control-panel`.

#### Install KFServing in 5 Minutes (On your local machine)

Make sure you have
[kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/#install-kubectl-on-linux),
[helm 3](https://helm.sh/docs/intro/install) installed before you start.(2 mins for setup)
1) If you do not have an existing kubernetes cluster you can create a quick kubernetes local cluster with [kind](https://github.com/kubernetes-sigs/kind#installation-and-usage).(this takes 30s)
```bash
kind create cluster
```
2) Install Istio lean version, Knative Serving, KFServing all in one.(this takes 30s)
```bash
./hack/quick_install.sh
```
#### Ingress Setup and Monitoring Stack
- [Configure Custom Ingress Gateway](https://knative.dev/docs/serving/setting-up-custom-ingress-gateway/)
  - In addition you need to update [KFServing configmap](config/default/configmap/inferenceservice.yaml) to use the custom ingress gateway.
- [Configure HTTPS Connection](https://knative.dev/docs/serving/using-a-tls-cert/)
- [Configure Custom Domain](https://knative.dev/docs/serving/using-a-custom-domain/)
- [Metrics](https://knative.dev/docs/serving/accessing-metrics/)
- [Tracing](https://knative.dev/docs/serving/accessing-traces/)
- [Logging](https://knative.dev/docs/serving/using-a-custom-domain/)
- [Dashboard for ServiceMesh](https://istio.io/latest/docs/tasks/observability/kiali/)

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
kubectl create namespace kfserving-test
kubectl apply -f docs/samples/sklearn/sklearn.yaml -n kfserving-test
```
3) Check KFServing `InferenceService` status.
```bash
kubectl get inferenceservices sklearn-iris -n kfserving-test
NAME           URL                                                              READY   DEFAULT TRAFFIC   CANARY TRAFFIC   AGE
sklearn-iris   http://sklearn-iris.kfserving-test.example.com/v1/models/sklearn-iris   True    100                                109s
```
4) Curl the `InferenceService`
```bash
kubectl port-forward --namespace istio-system $(kubectl get pod --namespace istio-system --selector="app=istio-ingressgateway" --output jsonpath='{.items[0].metadata.name}') 8080:80 &
SERVICE_HOSTNAME=$(kubectl get inferenceservice sklearn-iris -n kfserving-test -o jsonpath='{.status.url}' | cut -d "/" -f 3)
curl -v -H "Host: ${SERVICE_HOSTNAME}" http://localhost:8080/v1/models/sklearn-iris:predict -d @./docs/samples/sklearn/iris-input.json
```
5) Run Performance Test
```bash
kubectl create -f docs/samples/sklearn/perf.yaml -n kfserving-test
# wait the job to be done and check the log
kubectl logs load-test8b58n-rgfxr -n kfserving-test
Requests      [total, rate, throughput]         30000, 500.02, 499.99
Duration      [total, attack, wait]             1m0s, 59.998s, 3.336ms
Latencies     [min, mean, 50, 90, 95, 99, max]  1.743ms, 2.748ms, 2.494ms, 3.363ms, 4.091ms, 7.749ms, 46.354ms
Bytes In      [total, mean]                     690000, 23.00
Bytes Out     [total, mean]                     2460000, 82.00
Success       [ratio]                           100.00%
Status Codes  [code:count]                      200:30000  
Error Set:
```
 
### Use KFServing SDK
* Install the SDK
  ```
  pip install kfserving
  ```
* Get the KFServing SDK documents from [here](python/kfserving/README.md).

* Follow the [example here](docs/samples/client/kfserving_sdk_sample.ipynb) to use the KFServing SDK to create, rollout, promote, and delete an InferenceService instance.

### KFServing Features and Examples
[KFServing Features and Examples](./docs/samples/README.md)

### KFServing Roadmap
[KFServing Roadmap](./ROADMAP.md)

### KFServing Concepts and Data Plane
[KFServing Concepts and Data Plane](./docs/README.md)

### KFServing API Reference
[KFServing API Docs](./docs/apis/README.md)

### KFServing Debugging Guide :star:
[Debug KFServing InferenceService](./docs/KFSERVING_DEBUG_GUIDE.md)

### Developer Guide
[Developer Guide](/docs/DEVELOPER_GUIDE.md).

### Performance Tests
[KFServing benchmark test comparing Knative and Kubernetes Deployment with HPA](test/benchmark/README.md)
[Performance Tests](https://docs.google.com/document/d/1ss7M3cx1qD1PVpTaKTu_Y3C80JJz4nvMZlIyuZutZoE/edit#)

### Contributor Guide
[Contributor Guide](./CONTRIBUTING.md)

