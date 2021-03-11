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

### Architecture Review
[Control Plane and Data Plane](./docs/README.md)

### Core Features and Examples
[KFServing Features and Examples](./docs/samples/README.md)

### Learn More
To learn more about KFServing, how to deploy it as part of Kubeflow, how to use various supported features, and how to participate in the KFServing community, please follow the [KFServing docs on the Kubeflow Website](https://www.kubeflow.org/docs/components/serving/kfserving/). Additionally, we have compiled a list of [KFServing presentations and demoes](/docs/PRESENTATIONS.md) to dive through various details.

### Prerequisites

Kubernetes 1.16+ is the minimum recommended version for KFServing.

Knative Serving and Istio should be available on Kubernetes Cluster, KFServing currently depends on Istio Ingress Gateway to route requests to inference services.

- [Istio](https://knative.dev/docs/install/installing-istio): v1.3.1+

If you want to get up running Knative quickly or you do not need service mesh, we recommend installing Istio without service mesh(sidecar injection).
- [Knative Serving](https://knative.dev/docs/install/knative-with-any-k8s): v0.14.3+

`cluster-local-gateway` is required to serve cluster-internal traffic for transformer and explainer use cases. Please follow instructions here to install [cluster local gateway](https://knative.dev/docs/install/installing-istio/#updating-your-install-to-use-cluster-local-gateway).

If you are looking to use [PodSpec fields](https://v1-18.docs.kubernetes.io/docs/reference/generated/kubernetes-api/v1.18/#podspec-v1-core) such as `nodeSelector`, `affinity` or `tolerations` which are now supported in the KFServing v1beta1 API spec, this requires Knative v0.17.0+, and you need to turn on the corresponding [feature flags](https://knative.dev/docs/serving/feature-flags/) in your Knative configuration.

Since Knative v0.19.0 `cluster local gateway` deployment has been removed and [shared with ingress gateway](https://github.com/knative-sandbox/net-istio/pull/237),
if you are on Knative version later than v0.19.0 you should modify `localGateway` to `knative-local-gateway` and `localGatewayService` to `knative-local-gateway.istio-system.svc.cluster.local` in the
[inference service config](./config/configmap/inferenceservice.yaml).

- [Cert Manager](https://cert-manager.io/docs/installation/kubernetes): v0.12.0+

Cert manager is needed to provision KFServing webhook certs for production grade installation, alternatively you can run our self signed certs
generation [script](./hack/self-signed-ca.sh).


### Install KFServing
<details>
  <summary>Expand to see the installation options!</summary>

#### Standalone KFServing Installation
KFServing can be installed standalone if your kubernetes cluster meets the above prerequisites and KFServing controller is deployed in `kfserving-system` namespace.

```
TAG=v0.5.0
```

Install KFServing CRD

Due to [a performance issue applying deeply nested CRDs](https://github.com/kubernetes/kubernetes/issues/91615), please ensure that your `kubectl` version
fits into one of the following categories to ensure that you have the fix: `>=1.16.14,<1.17.0` or `>=1.17.11,<1.18.0` or `>=1.18.8`.
```shell
kubectl apply -f https://github.com/kubeflow/kfserving/releases/download/$TAG/kfserving_crds.yaml
```

Install KFServing Controller

```shell
kubectl apply -f https://github.com/kubeflow/kfserving/releases/download/$TAG/kfserving.yaml
```

#### Standalone KFServing on OpenShift

To install standalone KFServing on [OpenShift Container Platform](https://www.openshift.com/products/container-platform), please follow the [instructions here](docs/OPENSHIFT_GUIDE.md).

#### KFServing with Kubeflow Installation
KFServing is installed by default as part of Kubeflow installation using [Kubeflow manifests](https://github.com/kubeflow/manifests/tree/master/kfserving) and KFServing controller is deployed in `kubeflow` namespace.
Since Kubeflow Kubernetes minimal requirement is 1.14 which does not support object selector, `ENABLE_WEBHOOK_NAMESPACE_SELECTOR` is enabled in Kubeflow installation by default.
If you are using Kubeflow dashboard or [profile controller](https://www.kubeflow.org/docs/components/multi-tenancy/getting-started/#manual-profile-creation) to create  user namespaces, labels are automatically added to enable KFServing to deploy models.
If you are creating namespaces manually using Kubernetes apis directly, you will need to add label `serving.kubeflow.org/inferenceservice: enabled` to allow deploying KFServing `InferenceService` in the given namespaces, and do ensure you do not deploy
`InferenceService` in `kubeflow` namespace which is labelled as `control-plane`.

As of KFServing 0.4 release [object selector](https://kubernetes.io/docs/reference/access-authn-authz/extensible-admission-controllers/#matching-requests-objectselector) is turned on by default, the KFServing pod mutator is only invoked for KFServing `InferenceService` pods. For prior releases you can turn on manually by running following command.
```bash
kubectl patch mutatingwebhookconfiguration inferenceservice.serving.kubeflow.org --patch '{"webhooks":[{"name": "inferenceservice.kfserving-webhook-server.pod-mutator","objectSelector":{"matchExpressions":[{"key":"serving.kubeflow.org/inferenceservice", "operator": "Exists"}]}}]}'
```

#### Quick Install (On your local machine)

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
</details>

### Setup Ingress Gateway
If the default ingress gateway setup does not fit your need, you can choose to setup a custom ingress gateway
- [Configure Custom Ingress Gateway](https://knative.dev/docs/serving/setting-up-custom-ingress-gateway/)
  -  In addition you need to update [KFServing configmap](config/default/configmap/inferenceservice.yaml) to use the custom ingress gateway.
- [Configure Custom Domain](https://knative.dev/docs/serving/using-a-custom-domain/)
- [Configure HTTPS Connection](https://knative.dev/docs/serving/using-a-tls-cert/)

### Determine the ingress IP and ports
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

### Test KFServing Installation
<details>
  <summary>Expand to see steps for testing the installation!</summary>

#### Check KFServing controller installation
```shell
kubectl get po -n kfserving-system
NAME                             READY   STATUS    RESTARTS   AGE
kfserving-controller-manager-0   2/2     Running   2          13m
```

Please refer to our [troubleshooting section](docs/DEVELOPER_GUIDE.md#troubleshooting) for recommendations and tips for issues with installation.

#### Create KFServing test inference service
```bash
API_VERSION=v1beta1
kubectl create namespace kfserving-test
kubectl apply -f docs/samples/${API_VERSION}/sklearn/v1/sklearn.yaml -n kfserving-test
```
#### Check KFServing `InferenceService` status.
```bash
kubectl get inferenceservices sklearn-iris -n kfserving-test
NAME           URL                                                 READY   PREV   LATEST   PREVROLLEDOUTREVISION   LATESTREADYREVISION                    AGE
sklearn-iris   http://sklearn-iris.kfserving-test.example.com      True           100                              sklearn-iris-predictor-default-47q2g   7d23h
```
If your DNS contains example.com please consult your admin for configuring DNS or using [custom domain](https://knative.dev/docs/serving/using-a-custom-domain).

#### Curl the `InferenceService`
- Curl with real DNS

If you have configured the DNS, you can directly curl the `InferenceService` with the URL obtained from the status print.
e.g
```
curl -v http://sklearn-iris.kfserving-test.${CUSTOM_DOMAIN}/v1/models/sklearn-iris:predict -d @./docs/samples/${API_VERSION}/sklearn/v1/iris-input.json
```

- Curl with magic DNS

If you don't want to go through the trouble to get a real domain, you can instead use "magic" dns [xip.io](http://xip.io/).
The key is to get the external IP for your KFServing cluster.
```
kubectl get svc istio-ingressgateway --namespace istio-system
```
Look for the `EXTERNAL-IP` column's value(in this case 35.237.217.209)

```bash
NAME                   TYPE           CLUSTER-IP     EXTERNAL-IP      PORT(S)                                                                                                                                      AGE
istio-ingressgateway   LoadBalancer   10.51.253.94   35.237.217.209
```

Next step is to setting up the custom domain:
```bash
kubectl edit cm config-domain --namespace knative-serving
```

Now in your editor, change example.com to {{external-ip}}.xip.io (make sure to replace {{external-ip}} with the IP you found earlier).

With the change applied you can now directly curl the URL
```bash
curl -v http://sklearn-iris.kfserving-test.35.237.217.209.xip.io/v1/models/sklearn-iris:predict -d @./docs/samples/${API_VERSION}/sklearn/v1/iris-input.json
```

- Curl from ingress gateway with HOST Header

If you do not have DNS, you can still curl with the ingress gateway external IP using the HOST Header.
```bash
SERVICE_HOSTNAME=$(kubectl get inferenceservice sklearn-iris -n kfserving-test -o jsonpath='{.status.url}' | cut -d "/" -f 3)
curl -v -H "Host: ${SERVICE_HOSTNAME}" http://${INGRESS_HOST}:${INGRESS_PORT}/v1/models/sklearn-iris:predict -d @./docs/samples/${API_VERSION}/sklearn/v1/iris-input.json
```

- Curl from local cluster gateway

If you are calling from in cluster you can curl with the internal url with host {{InferenceServiceName}}.{{namespace}}
```bash
curl -v http://sklearn-iris.kfserving-test/v1/models/sklearn-iris:predict -d @./docs/samples/${API_VERSION}/sklearn/v1/iris-input.json
```

#### Run Performance Test
```bash
# use kubectl create instead of apply because the job template is using generateName which doesn't work with kubectl apply
kubectl create -f docs/samples/${API_VERSION}/sklearn/v1/perf.yaml -n kfserving-test
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
</details>

### Setup Monitoring
- [Prometheus based monitoring for KFServing](https://github.com/kubeflow/kfserving/blob/master/docs/samples/metrics-and-monitoring/README.md#install-prometheus)
- [Metrics driven automated rollouts using Iter8](https://github.com/kubeflow/kfserving/blob/master/docs/samples/metrics-and-monitoring/README.md#metrics-driven-experiments-and-progressive-delivery)
- [Dashboard for ServiceMesh](https://istio.io/latest/docs/tasks/observability/kiali/)

### Use KFServing SDK
* Install the SDK
  ```
  pip install kfserving
  ```
* Check the KFServing SDK documents from [here](python/kfserving/README.md).

* Follow the [example(s) here](docs/samples/client) to use the KFServing SDK to create, rollout, promote, and delete an InferenceService instance.

### KFServing Presentations and Demoes
[KFServing Presentations and Demoes](./docs/PRESENTATIONS.md)

### KFServing Roadmap
[KFServing Roadmap](./ROADMAP.md)

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
