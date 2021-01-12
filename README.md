# KFServing
[![go.dev reference](https://img.shields.io/badge/go.dev-reference-007d9c?logo=go&logoColor=white)](https://pkg.go.dev/github.com/kubeflow/kfserving)
[![Coverage Status](https://coveralls.io/repos/github/kubeflow/kfserving/badge.svg?branch=master)](https://coveralls.io/github/kubeflow/kfserving?branch=master)
[![Go Report Card](https://goreportcard.com/badge/github.com/kubeflow/kfserving)](https://goreportcard.com/report/github.com/kubeflow/kfserving)
[![Releases](https://img.shields.io/github/release-pre/kubeflow/kfserving.svg?sort=semver)](https://github.com/kubeflow/kfserving/releases)
[![LICENSE](https://img.shields.io/github/license/kubeflow/kfserving.svg)](https://github.com/kubeflow/kfserving/blob/master/LICENSE)
[![Slack Status](https://img.shields.io/badge/slack-join_chat-white.svg?logo=slack&style=social)](https://kubeflow.slack.com/join/shared_invite/zt-cpr020z4-PfcAue_2nw67~iIDy7maAQ)

KFServing provides a Kubernetes [Custom Resource Definition](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/) for serving machine learning (ML) models on arbitrary frameworks. It aims to solve production model serving use cases by providing performant, high abstraction interfaces for common ML frameworks like Tensorflow, XGBoost, ScikitLearn, PyTorch, and ONNX.

It encapsulates the complexity of autoscaling, networking, health checking, and server configuration to bring cutting edge serving features like GPU Autoscaling, Scale to Zero, and Canary Rollouts to your ML deployments. It enables a simple, pluggable, and complete story for Production ML Serving including prediction, pre-processing, post-processing and explainability. KFServing is being [used across various organizations.](./ADOPTERS.md)

![KFServing](/docs/diagrams/kfserving.png)

### Learn More
To learn more about KFServing, how to deploy it as part of Kubeflow, how to use various supported features, and how to participate in the KFServing community, please follow the [KFServing docs on the Kubeflow Website](https://www.kubeflow.org/docs/components/serving/kfserving/). Additionally, we have compiled a list of [KFServing presentations and demoes](/docs/PRESENTATIONS.md) to dive through various details.

### Prerequisites

Kubernetes 1.15+ is the minimum recommended version for KFServing.

Knative Serving and Istio should be available on Kubernetes Cluster, Knative depends on Istio Ingress Gateway to route requests to Knative services. To use the exact versions tested by the Kubeflow and KFServing teams, please refer to the [prerequisites on developer guide](docs/DEVELOPER_GUIDE.md#install-knative-on-a-kubernetes-cluster)

- [Istio](https://knative.dev/docs/install/installing-istio): v1.3.1+

If you want to get up running Knative quickly or you do not need service mesh, we recommend installing Istio without service mesh(sidecar injection).
- [Knative Serving](https://knative.dev/docs/install/knative-with-any-k8s): v0.14.3+

Currently only `Knative Serving` is required, `cluster-local-gateway` is required to serve cluster-internal traffic for transformer and explainer use cases. Please follow instructions here to install [cluster local gateway](https://knative.dev/docs/install/installing-istio/#updating-your-install-to-use-cluster-local-gateway)

- [Cert Manager](https://cert-manager.io/docs/installation/kubernetes): v0.12.0+

Cert manager is needed to provision KFServing webhook certs for production grade installation, alternatively you can run our self signed certs
generation [script](./hack/self-signed-ca.sh).

### Install KFServing
#### Standalone KFServing Installation
KFServing can be installed standalone if your kubernetes cluster meets the above prerequisites and KFServing controller is deployed in `kfserving-system` namespace.

For Kubernetes 1.16+ users
```
TAG=v0.4.1
kubectl apply -f ./install/$TAG/kfserving.yaml
```
For Kubernetes 1.15 users
```
TAG=v0.4.1
kubectl apply -f ./install/$TAG/kfserving.yaml --validate=false
```

KFServing uses pod mutator or [mutating admission webhooks](https://kubernetes.io/blog/2019/03/21/a-guide-to-kubernetes-admission-controllers/) to inject the storage initializer component of KFServing. By default all the pods in namespaces which are not labelled with `control-plane` label go through the pod mutator.
This can cause problems and interfere with Kubernetes control panel when KFServing pod mutator webhook is not in ready state yet.

As of KFServing 0.4 release [object selector](https://kubernetes.io/docs/reference/access-authn-authz/extensible-admission-controllers/#matching-requests-objectselector) is turned on by default, the KFServing pod mutator is only invoked for KFServing `InferenceService` pods. For prior releases you can turn on manually by running following command.
```bash
kubectl patch mutatingwebhookconfiguration inferenceservice.serving.kubeflow.org --patch '{"webhooks":[{"name": "inferenceservice.kfserving-webhook-server.pod-mutator","objectSelector":{"matchExpressions":[{"key":"serving.kubeflow.org/inferenceservice", "operator": "Exists"}]}}]}'
```

#### Standalone KFServing on OpenShift

To install standalone KFServing on [OpenShift Container Platform](https://www.openshift.com/products/container-platform), please follow the [instructions here](docs/OPENSHIFT_GUIDE.md).

#### KFServing with Kubeflow Installation
KFServing is installed by default as part of Kubeflow installation using [Kubeflow manifests](https://github.com/kubeflow/manifests/tree/master/kfserving) and KFServing controller is deployed in `kubeflow` namespace.
Since Kubeflow Kubernetes minimal requirement is 1.14 which does not support object selector, `ENABLE_WEBHOOK_NAMESPACE_SELECTOR` is enabled in Kubeflow installation by default.
If you are using Kubeflow dashboard or [profile controller](https://www.kubeflow.org/docs/components/multi-tenancy/getting-started/#manual-profile-creation) to create  user namespaces, labels are automatically added to enable KFServing to deploy models. If you are creating namespaces manually using Kubernetes apis directly, you will need to add label `serving.kubeflow.org/inferenceservice: enabled` to allow deploying KFServing `InferenceService` in the given namespaces, and do ensure you do not deploy
`InferenceService` in `kubeflow` namespace which is labelled as `control-plane`.

#### Install KFServing in 5 Minutes (On your local machine)

Make sure you have
[kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/#install-kubectl-on-linux) installed.

1) If you do not have an existing kubernetes cluster,
you can create a quick kubernetes local cluster with [kind](https://github.com/kubernetes-sigs/kind#installation-and-usage).

Note that the minimal requirement for running KFServing is 4 cpus and 8Gi memory,
so you need to change the [docker resource setting](https://docs.docker.com/docker-for-mac/#advanced) to use 4 cpus and 8Gi memory.
```bash
kind create cluster
```
alternatively you can use [Minikube](https://kubernetes.io/docs/setup/learning-environment/minikube)
```bash
minikube start --cpus 4 --memory 8192 --kubernetes-version=v1.17.11
```

2) Install Istio lean version, Knative Serving, KFServing all in one.(this takes 30s)
```bash
./hack/quick_install.sh
```

### Test KFServing Installation

#### Check KFServing controller installation
```shell
kubectl get po -n kfserving-system
NAME                             READY   STATUS    RESTARTS   AGE
kfserving-controller-manager-0   2/2     Running   2          13m
```

Please refer to our [troubleshooting section](docs/DEVELOPER_GUIDE.md#troubleshooting) for recommendations and tips for issues with installation.

#### Create KFServing test inference service
```bash
API_VERSION=v1alpha2
kubectl create namespace kfserving-test
kubectl apply -f docs/samples/${API_VERSION}/sklearn/sklearn.yaml -n kfserving-test
```
#### Check KFServing `InferenceService` status.
```bash
kubectl get inferenceservices sklearn-iris -n kfserving-test
NAME           URL                                                              READY   DEFAULT TRAFFIC   CANARY TRAFFIC   AGE
sklearn-iris   http://sklearn-iris.kfserving-test.example.com/v1/models/sklearn-iris   True    100                                109s
```

#### Determine the ingress IP and ports
Execute the following command to determine if your kubernetes cluster is running in an environment that supports external load balancers
```bash
$ kubectl get svc istio-ingressgateway -n istio-system
NAME                   TYPE           CLUSTER-IP       EXTERNAL-IP      PORT(S)   AGE
istio-ingressgateway   LoadBalancer   172.21.109.129   130.211.10.121   ...       17h
```
If the EXTERNAL-IP value is set, your environment has an external load balancer that you can use for the ingress gateway.

```bash
export INGRESS_HOST=$(kubectl -n istio-system get service istio-ingressgateway -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
export INGRESS_PORT=$(kubectl -n istio-system get service istio-ingressgateway -o jsonpath='{.spec.ports[?(@.name=="http2")].port}')
```

If the EXTERNAL-IP value is none (or perpetually pending), your environment does not provide an external load balancer for the ingress gateway. In this case, you can access the gateway using the service’s node port.
```bash
# GKE
export INGRESS_HOST=worker-node-address
# Minikube
export INGRESS_HOST=$(minikube ip)
# Other environment(On Prem)
export INGRESS_HOST=$(kubectl get po -l istio=ingressgateway -n istio-system -o jsonpath='{.items[0].status.hostIP}')

export INGRESS_PORT=$(kubectl -n istio-system get service istio-ingressgateway -o jsonpath='{.spec.ports[?(@.name=="http2")].nodePort}')
```

Alternatively you can do `Port Forward` for testing purpose
```bash
INGRESS_GATEWAY_SERVICE=$(kubectl get svc --namespace istio-system --selector="app=istio-ingressgateway" --output jsonpath='{.items[0].metadata.name}')
kubectl port-forward --namespace istio-system svc/${INGRESS_GATEWAY_SERVICE} 8080:80
# start another terminal
export INGRESS_HOST=localhost
export INGRESS_PORT=8080
```

#### Curl the `InferenceService`
Curl from ingress gateway
```bash
SERVICE_HOSTNAME=$(kubectl get inferenceservice sklearn-iris -n kfserving-test -o jsonpath='{.status.url}' | cut -d "/" -f 3)
curl -v -H "Host: ${SERVICE_HOSTNAME}" http://${INGRESS_HOST}:${INGRESS_PORT}/v1/models/sklearn-iris:predict -d @./docs/samples/${API_VERSION}/sklearn/iris-input.json
```
Curl from local cluster gateway
```bash
curl -v http://sklearn-iris.kfserving-test/v1/models/sklearn-iris:predict -d @./docs/samples/${API_VERSION}/sklearn/iris-input.json
```

#### Run Performance Test
```bash
# use kubectl create instead of apply because the job template is using generateName which doesn't work with kubectl apply
kubectl create -f docs/samples/${API_VERSION}/sklearn/perf.yaml -n kfserving-test
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

### Setup Ingress Gateway
If the default ingress gateway setup does not fit your need, you can choose to setup a custom ingress gateway
- [Configure Custom Ingress Gateway](https://knative.dev/docs/serving/setting-up-custom-ingress-gateway/)
  -  In addition you need to update [KFServing configmap](config/default/configmap/inferenceservice.yaml) to use the custom ingress gateway.
- [Configure Custom Domain](https://knative.dev/docs/serving/using-a-custom-domain/)
- [Configure HTTPS Connection](https://knative.dev/docs/serving/using-a-tls-cert/)

### Setup Monitoring
- [Metrics](https://knative.dev/docs/serving/accessing-metrics/)
- [Tracing](https://knative.dev/docs/serving/accessing-traces/)
- [Logging](https://knative.dev/docs/serving/accessing-logs/)
- [Dashboard for ServiceMesh](https://istio.io/latest/docs/tasks/observability/kiali/)

### Use KFServing SDK
* Install the SDK
  ```
  pip install kfserving
  ```
* Check the KFServing SDK documents from [here](python/kfserving/README.md).

* Follow the [example(s) here](docs/samples/client) to use the KFServing SDK to create, rollout, promote, and delete an InferenceService instance.

### KFServing Features and Examples
[KFServing Features and Examples](./docs/samples/README.md)

### KFServing Presentations and Demoes
[KFServing Presentations and Demoes](./docs/PRESENTATIONS.md)

### KFServing Roadmap
[KFServing Roadmap](./ROADMAP.md)

### KFServing Concepts and Data Plane
[KFServing Concepts and Data Plane](./docs/README.md)

### KFServing API Reference
[KFServing v1alpha2 API Docs](./docs/apis/v1alpha2/README.md)

[KFServing v1beta1 API Docs](./docs/apis/v1beta1/README.md)

### KFServing Debugging Guide :star:
[Debug KFServing InferenceService](./docs/KFSERVING_DEBUG_GUIDE.md)

### Developer Guide
[Developer Guide](/docs/DEVELOPER_GUIDE.md).

### Performance Tests
[KFServing benchmark test comparing Knative and Kubernetes Deployment with HPA](test/benchmark/README.md)

### Contributor Guide
[Contributor Guide](./CONTRIBUTING.md)

### KFServing Adopters
[KFServing Adopters](./ADOPTERS.md)
