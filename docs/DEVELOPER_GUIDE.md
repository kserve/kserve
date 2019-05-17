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

1. [`go`](https://golang.org/doc/install): KFServing controller is written in Go.
1. [`git`](https://help.github.com/articles/set-up-git/): For source control.
1. [`dep`](https://github.com/golang/dep): For managing external Go
   dependencies.
1. [`ko`](https://github.com/google/ko):
   For development.
1. [`kubectl`](https://kubernetes.io/docs/tasks/tools/install-kubectl/): For
   managing development environments.
1. [`kustomize`](https://github.com/kubernetes-sigs/kustomize/) To customize YAMLs for different environments

### Install Knative on a Kubernetes cluster

1. [Set up a kubernetes cluster and install Knative Serving](https://knative.dev/docs/install/)

- **Note**: KFServing currently only requires `Knative Serving` for auto-scaling, canary rollout,
 `Istio` for traffic routing and ingress. You can follow instructions on
 [Custom Install](https://knative.dev/docs/install/knative-custom-install) to install `Istio` and `Knative Serving`,
 observability plug-ins are good to have for monitoring.

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

```bash
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
and deploys the `kfserving-controller-manager` with the image digest to your cluster for testing. Please also ensure you are logged in to `KO_DOCKER_REPO` from your client machine.


### Smoke test after deployment
```bash
kubectl apply -f docs/samples/tensorflow.yaml
```
You should see model serving deployment running under default or your specified namespace.

```console
$ kubectl get pods -n default -l serving.kubeflow.org/kfservice=my-model
NAME                                                READY   STATUS    RESTARTS   AGE
my-model-default-htz8r-deployment-8fd979f9b-w2qbv   3/3     Running   0          10s
```

## Iterating

As you make changes to the code-base, there are two special cases to be aware
of:

- **If you change an input to generated code**, then you must run
  `make manifests`. Inputs include:

  - API type definitions in
    [pkg/apis/serving/v1alpha1/](../pkg/apis/serving/v1alpha1/.),
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

1. When you run make deploy, you may encounter an error like this:

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

2. When you run make deploy-dev, you may see an error like the one below:

```shell
2019/05/17 15:13:54 error processing import paths in "config/default/manager/manager.yaml": unsupported status code 401; body: 
kustomize build config/overlays/development | kubectl apply -f -
Error: reading strategic merge patches [manager_image_patch.yaml]: evalsymlink failure on '/Users/animeshsingh/go/src/github.com/kubeflow/kfserving/config/overlays/development/manager_image_patch.yaml' : lstat /Users/animeshsingh/go/src/github.com/kubeflow/kfserving/config/overlays/development/manager_image_patch.yaml: no such file or directory
```

It`s a red herring. To resolve it, please ensure you have logged into dockerhub from you client machine.