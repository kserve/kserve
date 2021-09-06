# KServe
[![go.dev reference](https://img.shields.io/badge/go.dev-reference-007d9c?logo=go&logoColor=white)](https://pkg.go.dev/github.com/kserve/kserve)
[![Coverage Status](https://coveralls.io/repos/github/kserve/kserve/badge.svg?branch=master)](https://coveralls.io/github/kserve/kserve?branch=master)
[![Go Report Card](https://goreportcard.com/badge/github.com/kserve/kserve)](https://goreportcard.com/report/github.com/kserve/kserve)
[![Releases](https://img.shields.io/github/release-pre/kserve/kserve.svg?sort=semver)](https://github.com/kserve/kserve/releases)
[![LICENSE](https://img.shields.io/github/license/kserve/kserve.svg)](https://github.com/kserve/kserve/blob/master/LICENSE)
[![Slack Status](https://img.shields.io/badge/slack-join_chat-white.svg?logo=slack&style=social)](https://kubeflow.slack.com/join/shared_invite/zt-cpr020z4-PfcAue_2nw67~iIDy7maAQ)

KServe provides a Kubernetes [Custom Resource Definition](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/) for serving machine learning (ML) models on arbitrary frameworks. It aims to solve production model serving use cases by providing performant, high abstraction interfaces for common ML frameworks like Tensorflow, XGBoost, ScikitLearn, PyTorch, and ONNX.

It encapsulates the complexity of autoscaling, networking, health checking, and server configuration to bring cutting edge serving features like GPU Autoscaling, Scale to Zero, and Canary Rollouts to your ML deployments. It enables a simple, pluggable, and complete story for Production ML Serving including prediction, pre-processing, post-processing and explainability. KServe is being [used across various organizations.](./ADOPTERS.md)

![KServe](/docs/diagrams/kfserving.png)

### Architecture Review
[Control Plane and Data Plane](./docs/README.md)

### Core Features and Examples
[Features and Examples](./docs/samples/README.md)

### Learn More
To learn more about KServe, how to deploy it as part of Kubeflow, how to use various supported features, and how to participate in the KServe community, please follow the [KFServing docs on the Kubeflow Website](https://www.kubeflow.org/docs/components/serving/kfserving/). Additionally, we have compiled a list of [presentations and demoes](/docs/PRESENTATIONS.md) to dive through various details.

### Prerequisites

Kubernetes 1.17 is the minimally recommended version, Knative Serving and Istio should be available on Kubernetes Cluster.

- [Istio](https://knative.dev/docs/install/installing-istio): v1.9.0+
   * KServe currently only depends on `Istio Ingress Gateway` to route requests to inference services externally or internally.
     If you do not need `Service Mesh`, we recommend turning off Istio sidecar injection.

- [Knative Serving](https://knative.dev/docs/install): v0.19.0+
   * If you are running `Service Mesh` mode with `Authorization` please follow knative doc to [setup the authorization policies](https://knative.dev/docs/serving/istio-authorization).
   * If you are looking to use [PodSpec fields](https://v1-18.docs.kubernetes.io/docs/reference/generated/kubernetes-api/v1.18/#podspec-v1-core) such as `nodeSelector`, `affinity` or `tolerations` which are now supported in the v1beta1 API spec,
   you need to turn on the corresponding [feature flags](https://knative.dev/docs/serving/feature-flags/) in your Knative configuration.

- [Cert Manager](https://cert-manager.io/docs/installation/kubernetes): v1.3.0+
   * Cert manager is needed to provision webhook certs for production grade installation, alternatively you can run our self signed certs
generation [script](./hack/self-signed-ca.sh).


### Installation

#### Standalone Installation
KServe can be installed standalone if your kubernetes cluster meets the above prerequisites and is deployed in `kserve` namespace.

```
TAG=v0.7.0-rc0
```

Install KServe CRD and Controller

Due to [a performance issue applying deeply nested CRDs](https://github.com/kubernetes/kubernetes/issues/91615), please ensure that your `kubectl` version
fits into one of the following categories to ensure that you have the fix: `>=1.16.14,<1.17.0` or `>=1.17.11,<1.18.0` or `>=1.18.8`.
```shell
kubectl apply -f https://github.com/kserve/kserve/releases/download/$TAG/kserve.yaml
```

#### Quick Install (On your local machine)

Make sure you have
[kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/#install-kubectl-on-linux) installed.

1) If you do not have an existing kubernetes cluster,
you can create a quick kubernetes local cluster with [kind](https://github.com/kubernetes-sigs/kind#installation-and-usage).

Note that the minimal requirement for running KServe is 4 cpus and 8Gi memory,
so you need to change the [docker resource setting](https://docs.docker.com/docker-for-mac/#advanced) to use 4 cpus and 8Gi memory.
```bash
kind create cluster
```
alternatively you can use [Minikube](https://kubernetes.io/docs/setup/learning-environment/minikube)
```bash
minikube start --cpus 4 --memory 8192
```

2) Install Istio lean version, Knative Serving, KServe all in one.(this takes 30s)
```bash
./hack/quick_install.sh
```

### Setup Ingress Gateway
If the default ingress gateway setup does not fit your need, you can choose to setup a custom ingress gateway
- [Configure Custom Ingress Gateway](https://knative.dev/docs/serving/setting-up-custom-ingress-gateway/)
  -  In addition you need to update [configmap](config/configmap/inferenceservice.yaml) to use the custom ingress gateway.
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

### Test Installation
<details>
  <summary>Expand to see steps for testing the installation!</summary>

#### Verify installation
```shell
kubectl get po -n kserve
NAME                             READY   STATUS    RESTARTS   AGE
kserve-controller-manager-0   2/2     Running   2          13m
```

Please refer to our [troubleshooting section](docs/DEVELOPER_GUIDE.md#troubleshooting) for recommendations and tips for issues with installation.

#### Create test inference service
```bash
API_VERSION=v1beta1
kubectl create namespace kserve-test
kubectl apply -f docs/samples/${API_VERSION}/sklearn/v1/sklearn.yaml -n kserve-test
```
#### Check `InferenceService` status.
```bash
kubectl get inferenceservices sklearn-iris -n kserve-test
NAME           URL                                                 READY   PREV   LATEST   PREVROLLEDOUTREVISION   LATESTREADYREVISION                    AGE
sklearn-iris   http://sklearn-iris.kserve-test.example.com         True           100                              sklearn-iris-predictor-default-47q2g   7d23h
```
If your DNS contains example.com please consult your admin for configuring DNS or using [custom domain](https://knative.dev/docs/serving/using-a-custom-domain).

#### Curl the `InferenceService`
- Curl with real DNS

If you have configured the DNS, you can directly curl the `InferenceService` with the URL obtained from the status print.
e.g
```
curl -v http://sklearn-iris.kserve-test.${CUSTOM_DOMAIN}/v1/models/sklearn-iris:predict -d @./docs/samples/${API_VERSION}/sklearn/v1/iris-input.json
```

- Curl with magic DNS

If you don't want to go through the trouble to get a real domain, you can instead use "magic" dns [xip.io](http://xip.io/).
The key is to get the external IP for your cluster.
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
curl -v http://sklearn-iris.kserve-test.35.237.217.209.xip.io/v1/models/sklearn-iris:predict -d @./docs/samples/${API_VERSION}/sklearn/v1/iris-input.json
```

- Curl from ingress gateway with HOST Header

If you do not have DNS, you can still curl with the ingress gateway external IP using the HOST Header.
```bash
SERVICE_HOSTNAME=$(kubectl get inferenceservice sklearn-iris -n kserve-test -o jsonpath='{.status.url}' | cut -d "/" -f 3)
curl -v -H "Host: ${SERVICE_HOSTNAME}" http://${INGRESS_HOST}:${INGRESS_PORT}/v1/models/sklearn-iris:predict -d @./docs/samples/${API_VERSION}/sklearn/v1/iris-input.json
```

- Curl from local cluster gateway

If you are calling from in cluster you can curl with the internal url with host {{InferenceServiceName}}.{{namespace}}
```bash
curl -v http://sklearn-iris.kserve-test/v1/models/sklearn-iris:predict -d @./docs/samples/${API_VERSION}/sklearn/v1/iris-input.json
```

#### Run Performance Test
```bash
# use kubectl create instead of apply because the job template is using generateName which doesn't work with kubectl apply
kubectl create -f docs/samples/${API_VERSION}/sklearn/v1/perf.yaml -n kserve-test
# wait the job to be done and check the log
kubectl logs load-test8b58n-rgfxr -n kserve-test
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
- [Prometheus based monitoring](https://github.com/kserve/kserve/blob/master/docs/samples/metrics-and-monitoring/README.md#install-prometheus)
- [Metrics driven automated rollouts using Iter8](https://iter8.tools)
- [Dashboard for ServiceMesh](https://istio.io/latest/docs/tasks/observability/kiali/)

### Use KServe SDK
* Install the SDK
  ```
  pip install kserve
  ```
* Check the SDK documents from [here](python/kserve/README.md).

* Follow the [example(s) here](docs/samples/client) to use the KServe SDK to create, rollout, promote, and delete an InferenceService instance.

### Presentations and Demoes
[Presentations and Demoes](./docs/PRESENTATIONS.md)

### Roadmap
[Roadmap](./ROADMAP.md)

### API Reference
[InferenceService v1beta1 API Docs](./docs/apis/v1beta1/README.md)


### Debugging Guide :star:
[Debug InferenceService](./docs/KFSERVING_DEBUG_GUIDE.md)

### Developer Guide
[Developer Guide](/docs/DEVELOPER_GUIDE.md).

### Performance Tests
[benchmark test comparing Knative and Kubernetes Deployment with HPA](test/benchmark/README.md)

### Contributor Guide
[Contributor Guide](./CONTRIBUTING.md)

### Adopters
[Adopters](./ADOPTERS.md)
