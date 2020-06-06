<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->
**Table of Contents**  *generated with [DocToc](https://github.com/thlorenz/doctoc)*

- [Development](#development)
  - [Prerequisites](#prerequisites)
    - [Install requirements](#install-requirements)
    - [Install Knative on a Kubernetes cluster](#install-knative-on-a-kubernetes-cluster)
    - [Setup your environment](#setup-your-environment)
    - [Checkout your fork](#checkout-your-fork)
  - [Deploy KFServing](#deploy-kfserving)
    - [Check Knative Serving installation](#check-knative-serving-installation)
    - [Deploy KFServing from default](#deploy-kfserving-from-default)
    - [Deploy KFServing with your own version](#deploy-kfserving-with-your-own-version)
    - [Smoke test after deployment](#smoke-test-after-deployment)
  - [Iterating](#iterating)
  - [Troubleshooting](#troubleshooting)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

# Development

This doc explains how to setup a development environment so you can get started
[contributing](../CONTRIBUTING.md)
to `kfserving`. Also take a look at:

- [How to add and run tests](../test/README.md)
- [Iterating](#iterating)

## Prerequisites

Follow the instructions below to set up your development environment. Once you
meet these requirements, you can make changes and
[deploy your own version of kfserving](#deploy-kfserving)!

Before submitting a PR, see also [CONTRIBUTING.md](../CONTRIBUTING.md).

### Install requirements

You must install these tools:

1. [`go`](https://golang.org/doc/install): KFServing controller is written in Go and requires Go 1.13.0+.
1. [`git`](https://help.github.com/articles/set-up-git/): For source control.
1. [`dep`](https://github.com/golang/dep): For managing external Go
   dependencies. You should install `dep` using their `install.sh`.
1. [`ko`](https://github.com/google/ko):
   For development.
1. [`kubectl`](https://kubernetes.io/docs/tasks/tools/install-kubectl/): For
   managing development environments.
1. [`kustomize`](https://github.com/kubernetes-sigs/kustomize/) To customize YAMLs for different environments, requires v3.5.4+.

### Install Knative on a Kubernetes cluster

KFServing currently requires `Knative Serving` for auto-scaling, canary rollout, `Istio` for traffic routing and ingress.

* You can follow the instructions on [Set up a kubernetes cluster and install Knative Serving](https://knative.dev/docs/install/) or
[Custom Install](https://knative.dev/docs/install/knative-custom-install) to install `Istio` and `Knative Serving`. Observability plug-ins are good to have for monitoring.

* If you already have `Istio` or `Knative` (e.g. from a Kubeflow install) then you don't need to install them explictly, as long as version dependencies are satisfied. With Kubeflow v0.7, KNative 0.8 and Istio 1.1.6 are installed by default as part of the Kubeflow installation. From Kubeflow 1.0 onwards, KNative 0.11.1 and Istio 1.1.6 are installed by default. If you are using DEX based config for Kubeflow 1.0, Istio 1.3.1 is installed by default in your Kubeflow cluster. To summarize, we would recommend KNative 0.11.1 at a minimum for KFServing 0.3.0 and for the KFServing code in master. For Istio use versions 1.1.6 and 1.3.1 which have been tested, and for Kubernetes use 1.15+ 

* You need follow the instructions on [Updating your install to use cluster local gateway](https://knative.dev/docs/install/installing-istio/#updating-your-install-to-use-cluster-local-gateway) to add cluster local gateway to your cluster to serve cluster-internal traffic for transformer and explainer use cases.

### Setup your environment

To start your environment you'll need to set these environment variables (we
recommend adding them to your `.bashrc`):

1. `GOPATH`: If you don't have one, simply pick a directory and add
   `export GOPATH=...`
1. `$GOPATH/bin` on `PATH`: This is so that tooling installed via `go get` will
   work properly.
1. `KO_DOCKER_REPO`: The docker repository to which developer images should be
   pushed (e.g. `docker.io/<username>/[project]`).

- **Note**: Set up a docker repository for pushing images. You can use any container image registry by adjusting 
the authentication methods and repository paths mentioned in the sections below.
   - [Google Container Registry quickstart](https://cloud.google.com/container-registry/docs/pushing-and-pulling)
   - [Docker Hub quickstart](https://docs.docker.com/docker-hub/)
   - [Azure Container Registry quickstart](https://docs.microsoft.com/en-us/azure/container-registry/container-registry-get-started-portal)
- **Note**: if you are using docker hub to store your images your
  `KO_DOCKER_REPO` variable should be `docker.io/<username>`.
- **Note**: Currently Docker Hub doesn't let you create subdirs under your
  username.

`.bashrc` example:

```shell
export GOPATH="$HOME/go"
export PATH="${PATH}:${GOPATH}/bin"
export KO_DOCKER_REPO='docker.io/<username>'
```

### Checkout your fork

The Go tools require that you clone the repository to the
`src/github.com/kubeflow/kfserving` directory in your
[`GOPATH`](https://github.com/golang/go/wiki/SettingGOPATH).

To check out this repository:

1. Create your own
   [fork of this repo](https://help.github.com/articles/fork-a-repo/)
1. Clone it to your machine:

```shell
mkdir -p ${GOPATH}/src/github.com/kubeflow
cd ${GOPATH}/src/github.com/kubeflow
git clone git@github.com:${YOUR_GITHUB_USERNAME}/kfserving.git
cd kfserving
git remote add upstream git@github.com:kubeflow/kfserving.git
git remote set-url --push upstream no_push
```

_Adding the `upstream` remote sets you up nicely for regularly
[syncing your fork](https://help.github.com/articles/syncing-a-fork/)._

Once you reach this point you are ready to do a full build and deploy as
described below.

## Deploy KFServing

### Check Knative Serving installation
Once you've [setup your development environment](#prerequisites), you can see things running with:

```console
$ kubectl -n knative-serving get pods
NAME                          READY     STATUS    RESTARTS   AGE
activator-c8495dc9-z7xpz      2/2       Running   0          6d
autoscaler-66897845df-t5cwg   2/2       Running   0          6d
controller-699fb46bb5-xhlkg   1/1       Running   0          6d
webhook-76b87b8459-tzj6r      1/1       Running   0          6d
```
### Deploy KFServing from default
We suggest using [cert manager](https://github.com/jetstack/cert-manager) for
provisioning the certificates for the KFServing webhook server. Other solutions should
also work as long as they put the certificates in the desired location.

You can follow
[the cert manager documentation](https://docs.cert-manager.io/en/latest/getting-started/install/kubernetes.html)
to install it.

If you don't want to install cert manager, you can set the `KFSERVING_ENABLE_SELF_SIGNED_CA` environment variable to true.
`KFSERVING_ENABLE_SELF_SIGNED_CA` will execute a script to create a self-signed CA and patch it to the webhook config.
```bash
export KFSERVING_ENABLE_SELF_SIGNED_CA=true
```

After that you can run following command to deploy `KFServing`, you can skip above step once cert manager is installed.
```bash
make deploy
```

**Optional**: you can set CPU and memory limits when deploying `KFServing`.
```bash
make deploy KFSERVING_CONTROLLER_CPU_LIMIT=<cpu_limit> KFSERVING_CONTROLLER_MEMORY_LIMIT=<memory_limit>
```

or
```bash
export KFSERVING_CONTROLLER_CPU_LIMIT=<cpu_limit>
export KFSERVING_CONTROLLER_MEMORY_LIMIT=<memory_limit>
make deploy
```

After above step you can see things running with:
```console
$ kubectl get pods -n kfserving-system -l control-plane=kfserving-controller-manager
NAME                             READY   STATUS    RESTARTS   AGE
kfserving-controller-manager-0   2/2     Running   0          13m
```
- **Note**: By default it installs to `kfserving-system` namespace with the published
`kfserving-controller-manager` image.

### Deploy KFServing with your own version
```bash
make deploy-dev
```
- **Note**: `deploy-dev` builds the image from your local code, publishes to `KO_DOCKER_REPO`
and deploys the `kfserving-controller-manager` and `logger` with the image digest to your cluster for testing. Please also ensure you are logged in to `KO_DOCKER_REPO` from your client machine.

There's also commands to build and deploy the image from your local code for the models, explainer and storage-initializer, you can choose to use one of below commands for your development purpose.
```bash
make deploy-dev-sklearn

make deploy-dev-xgb

make deploy-dev-pytorch

make deploy-dev-alibi

make deploy-dev-storageInitializer
```
- **Note**: These commands also publishes to `KO_DOCKER_REPO` with the image of version 'latest', and change the configmap of your cluster to point to the new built images. It's just for development and testing purpose so you need to do it one by one. In configmap, for predictors it will just keep the one in development, for exlainer and storage initializer will just change the item impacted and set all others images including the `kfserving-controller-manager` and `logger` to be default. 

### Smoke test after deployment
```bash
kubectl apply -f docs/samples/tensorflow/tensorflow.yaml
```
You should see model serving deployment running under default or your specified namespace.

```console
$ kubectl get inferenceservices -n default
NAME             READY     URL                                  DEFAULT TRAFFIC   CANARY TRAFFIC   AGE
flowers-sample   True      flowers-sample.default.example.com   100                                1h

$ kubectl get pods -n default -l serving.kubeflow.org/inferenceservice=flowers-sample
NAME                                                READY   STATUS    RESTARTS   AGE
flowers-sample-default-htz8r-deployment-8fd979f9b-w2qbv   3/3     Running   0          10s
```
NOTE: KFServing scales pods to 0 in the absence of traffic. If you don't see any pods, try sending out a query via curl using instructions in the tensorflow sample: https://github.com/kubeflow/kfserving/tree/master/docs/samples/tensorflow


## Iterating

As you make changes to the code-base, there are two special cases to be aware
of:

- **If you change an input to generated code**, then you must run
  `make manifests`. Inputs include:

  - API type definitions in
    [pkg/apis/serving/v1alpha2/](../pkg/apis/serving/v1alpha2/.),
  - Manifests or kustomize patches stored in [config](../config).

- **If you change a package's deps** (including adding external dep), then you
  must run `dep ensure`. Dependency changes should be a separate commit and not
  mixed with logic changes.

These are both idempotent, and we expect that running these at `HEAD` to have no
diffs. Code generation and dependencies are automatically checked to produce no
diffs for each pull request.

In some cases, if newer dependencies
are required, you need to run "dep ensure -update package-name" manually.

Once the codegen and dependency information is correct, redeploying the
controller is simply:

```shell
make deploy-dev
```
## Troubleshooting

1. If you are on kubernetes 1.15+, we highly recommend adding object selector on kfserving pod mutating webhook configuration so that only pods managed by kfserving go through the kfserving pod mutator

```
kubectl patch mutatingwebhookconfiguration inferenceservice.serving.kubeflow.org --patch '{"webhooks":[{"name": "inferenceservice.kfserving-webhook-server.pod-mutator","objectSelector":{"matchExpressions":[{"key":"serving.kubeflow.org/inferenceservice", "operator": "Exists"}]}}]}'
```

2. You may get one of the following errors after deploying KFServing

```shell
Reissued from statefulset/default: create Pod default-0 in StatefulSet default failed error: Internal error occurred: failed calling webhook "inferenceservice.kfserving-webhook-server.pod.mutator": Post https://kfserving-webhook-server-service.kubeflow.svc:443/mutate-pods?timeout=30s: service "kfserving-webhook-service" not found
```

Or while you are deploying the models

```shell
kubectl apply -f docs/samples/tensorflow/tensorflow.yaml
Error from server (InternalError): error when creating "docs/samples/tensorflow/tensorflow.yaml": 
Internal error occurred: failed calling webhook "inferenceservice.kfserving-webhook-server.defaulter": 
Post https://kfserving-webhook-server-service.kfserving-system.svc:443/mutate-inferenceservices?timeout=30s:

context deadline exceeded
unexpected EOF
dial tcp x.x.x.x:443: connect: connection refused
```

If above errors appear, first thing to check is if the KFServing controller is running

```shell
kubectl get po -n kfserving-system
NAME                             READY   STATUS    RESTARTS   AGE
kfserving-controller-manager-0   2/2     Running   2          13m
```

If it is, more often than not, it is caused by a stale webhook, since webhooks are immutable. Even if the KFServing controller is not running, you might have stale webhooks from last deployment causing other issues. Best is to delete them, and test again


```shell
kubectl delete mutatingwebhookconfigurations inferenceservice.serving.kubeflow.org &&  kubectl delete validatingwebhookconfigurations inferenceservice.serving.kubeflow.org && kubectl delete po kfserving-controller-manager-0  -n kfserving-system

mutatingwebhookconfiguration.admissionregistration.k8s.io "inferenceservice.serving.kubeflow.org" deleted
validatingwebhookconfiguration.admissionregistration.k8s.io "inferenceservice.serving.kubeflow.org" deleted
```

3. When you run make deploy, you may encounter an error like this:

```shell
error: error validating "STDIN": error validating data: ValidationError(CustomResourceDefinition.spec.validation.openAPIV3Schema.properties.status.properties.conditions.properties.conditions.items): invalid type for io.k8s.apiextensions-apiserver.pkg.apis.apiextensions.v1beta1.JSONSchemaPropsOrArray: got "map", expected ""; if you choose to ignore these errors, turn validation off with --validate=false
make: *** [deploy] Error 1
```

To fix it, please ensure you have a matching version of kubectl client as the master. If not, please update accordingly.

```shell
kubectl version
Client Version: version.Info{Major:"1", Minor:"13", GitVersion:"v1.13.6", GitCommit:"abdda3f9fefa29172298a2e42f5102e777a8ec25", GitTreeState:"clean", BuildDate:"2019-05-08T13:53:53Z", GoVersion:"go1.11.5", Compiler:"gc", Platform:"darwin/amd64"}
Server Version: version.Info{Major:"1", Minor:"13", GitVersion:"v1.13.6+IKS", GitCommit:"ac5f7341d5d0ce8ea8f206ba5b030dc9e9d4cc97", GitTreeState:"clean", BuildDate:"2019-05-09T13:26:51Z", GoVersion:"go1.11.5", Compiler:"gc", Platform:"linux/amd64"}
```

4. When you run make deploy-dev, you may see an error like the one below:

```shell
2019/05/17 15:13:54 error processing import paths in "config/default/manager/manager.yaml": unsupported status code 401; body: 
kustomize build config/overlays/development | kubectl apply -f -
Error: reading strategic merge patches [manager_image_patch.yaml]: evalsymlink failure on '/Users/animeshsingh/go/src/github.com/kubeflow/kfserving/config/overlays/development/manager_image_patch.yaml' : lstat /Users/animeshsingh/go/src/github.com/kubeflow/kfserving/config/overlays/development/manager_image_patch.yaml: no such file or directory
```

It`s a red herring. To resolve it, please ensure you have logged into dockerhub from you client machine.

5. When you deploy the tensorflow sample, you may encounter an error like the one blow:

```
2019-09-28 01:52:23.345692: E tensorflow_serving/sources/storage_path/file_system_storage_path_source.cc:362] FileSystemStoragePathSource encountered a filesystem access error: Could not find base path /mnt/models for servable flowers-sample
```

Please make sure not to deploy the inferenceservice in the `kfserving-system` or other namespaces where namespace has  `control-plane` as a label. The `storage-initializer` init container does not get injected for deployments in those namespaces since they do not go through the mutating webhook.

6. When you deploy the tensorflow sample, you may get `IngressNotConfigured` error:

This often happens when KNative fails to probe the Istio ingress gateway to your inference service and you may find the HTTP error code in KNative `network-istio` pod logs.

If you are seeing HTTP 401 or 302, then you may have Auth turned on for `Istio Ingress Gateway` which blocks the Knative probes to the your service.

```shell
kubectl logs -l app=networking-istio -n knative-serving

[2020-02-11T18:16:21.419Z] "GET / HTTP/1.1" 404 NR "-" "-" 0 0 0 - "10.88.0.31" "Go-http-client/1.1" "4a8bd584-2323-4f40-9230-9797d890b9fb" "helloworld-go.default:80" "-" - - 10.88.1.13:80 10.88.0.31:36237 - -
[2020-02-11T18:16:21.419Z] "GET / HTTP/1.1" 404 NR "-" "-" 0 0 0 - "10.88.0.31" "Go-http-client/1.1" "7298dbfc-58bb-430f-92c5-cf39e97f63d7" "helloworld-go.default.svc:80" "-" - - 10.88.1.13:80 10.88.0.31:36239 - -
[2020-02-11T18:16:21.420Z] "GET / HTTP/1.1" 302 UAEX "-" "-" 0 269 21 21 "10.88.0.31" "Go-http-client/1.1" "27aa43fa-ac17-4a71-8ca2-b4d9fb772219" "helloworld-go.default.example.com:80" "-" - - 10.88.1.13:80 10.88.0.31:36249 - -
```

If you are seeing HTTP 403, then you may have `Istio RBAC` turned on which blocks the probes to your service, you can create Istio RBAC rule to allow the probes from `knative-serving` namespace or disable the istio sidecar injection by adding the `sidecar.istio.io/inject: false` annotation to the inference service.

```json
{"level":"error","ts":"2020-03-26T19:12:00.749Z","logger":"istiocontroller.ingress-controller.status-manager","caller":"ingress/status.go:366",
"msg":"Probing of http://flowers-sample-predictor-default.kubeflow-jeanarmel-luce.example.com:80/ failed, IP: 10.0.0.29:80, ready: false, error: unexpected status code: want [200], got 403 (depth: 0)",
"commit":"6b0e5c6","knative.dev/controller":"ingress-controller","stacktrace":"knative.dev/serving/pkg/reconciler/ingress.(*StatusProber).processWorkItem\n\t/home/prow/go/src/knative.dev/serving/pkg/reconciler/ingress/status.go:366\nknative.dev/serving/pkg/reconciler/ingress.(*StatusProber).Start.func1\n\t/home/prow/go/src/knative.dev/serving/pkg/reconciler/ingress/status.go:268"}
``` 

KNative has been addressing the probe issue in https://github.com/knative/serving/issues/6829 with best effort probes and KFServing has a temporary solution with KFServing Ingress Gateway in Kubeflow manifests until the fix is released, meanwhile we are working a proper AuthN/AuthZ story for KFServing in https://github.com/kubeflow/kfserving/issues/760.
