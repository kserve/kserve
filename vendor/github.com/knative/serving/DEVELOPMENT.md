# Development

This doc explains how to setup a development environment so you can get started
[contributing](https://www.knative.dev/contributing/) to `Knative Serving`. Also
take a look at:

- [The pull request workflow](https://www.knative.dev/contributing/contributing/#pull-requests)
- [How to add and run tests](./test/README.md)
- [Iterating](#iterating)

## Prerequisites

Follow the instructions below to set up your development environment. Once you
meet these requirements, you can make changes and
[deploy your own version of Knative Serving](#starting-knative-serving)!

Before submitting a PR, see also [CONTRIBUTING.md](./CONTRIBUTING.md).

### Sign up for GitHub

Start by creating [a GitHub account](https://github.com/join), then setup
[GitHub access via SSH](https://help.github.com/articles/connecting-to-github-with-ssh/).

### Install requirements

You must install these tools:

1. [`go`](https://golang.org/doc/install): The language `Knative Serving` is
   built in (1.12rc1 or later)
1. [`git`](https://help.github.com/articles/set-up-git/): For source control
1. [`dep`](https://github.com/golang/dep): For managing external Go
   dependencies.
1. [`ko`](https://github.com/google/ko): For development.
1. [`kubectl`](https://kubernetes.io/docs/tasks/tools/install-kubectl/): For
   managing development environments.

### Create a cluster and a repo

1. [Set up a kubernetes cluster](https://www.knative.dev/docs/install/)
   - Follow an install guide up through "Creating a Kubernetes Cluster"
   - You do _not_ need to install Istio or Knative using the instructions in the
     guide. Simply create the cluster and come back here.
   - If you _did_ install Istio/Knative following those instructions, that's
     fine too, you'll just redeploy over them, below.
1. Set up a docker repository for pushing images. You can use any container
   image registry by adjusting the authentication methods and repository paths
   mentioned in the sections below.
   - [Google Container Registry quickstart](https://cloud.google.com/container-registry/docs/pushing-and-pulling)
   - [Docker Hub quickstart](https://docs.docker.com/docker-hub/)

**Note**: You'll need to be authenticated with your `KO_DOCKER_REPO` before
pushing images. Run `gcloud auth configure-docker` if you are using Google
Container Registry or `docker login` if you are using Docker Hub.

### Setup your environment

To start your environment you'll need to set these environment variables (we
recommend adding them to your `.bashrc`):

1. `GOPATH`: If you don't have one, simply pick a directory and add
   `export GOPATH=...`
1. `$GOPATH/bin` on `PATH`: This is so that tooling installed via `go get` will
   work properly.
1. `KO_DOCKER_REPO`: The docker repository to which developer images should be
   pushed (e.g. `gcr.io/[gcloud-project]`).

- **Note**: if you are using docker hub to store your images your
  `KO_DOCKER_REPO` variable should be `docker.io/<username>`.
- **Note**: Currently Docker Hub doesn't let you create subdirs under your
  username.

`.bashrc` example:

```shell
export GOPATH="$HOME/go"
export PATH="${PATH}:${GOPATH}/bin"
export KO_DOCKER_REPO='gcr.io/my-gcloud-project-id'
```

### Checkout your fork

The Go tools require that you clone the repository to the
`src/github.com/knative/serving` directory in your
[`GOPATH`](https://github.com/golang/go/wiki/SettingGOPATH).

To check out this repository:

1. Create your own
   [fork of this repo](https://help.github.com/articles/fork-a-repo/)
1. Clone it to your machine:

```shell
mkdir -p ${GOPATH}/src/github.com/knative
cd ${GOPATH}/src/github.com/knative
git clone git@github.com:${YOUR_GITHUB_USERNAME}/serving.git
cd serving
git remote add upstream git@github.com:knative/serving.git
git remote set-url --push upstream no_push
```

_Adding the `upstream` remote sets you up nicely for regularly
[syncing your fork](https://help.github.com/articles/syncing-a-fork/)._

Once you reach this point you are ready to do a full build and deploy as
described below.

## Starting Knative Serving

Once you've [setup your development environment](#prerequisites), stand up
`Knative Serving`. Note that if you already installed Knative to your cluster,
redeploying the new version should work fine, but if you run into trouble, you
can easily [clean your cluster up](#clean-up) and try again.

### Setup cluster admin

Your user must be a cluster admin to perform the setup needed for Knative.

The value you use depends on
[your cluster setup](https://www.knative.dev/docs/install/): when using
Minikube, the user is your local user; when using GKE, the user is your GCP
user.

```shell
# For GCP
kubectl create clusterrolebinding cluster-admin-binding \
  --clusterrole=cluster-admin \
  --user=$(gcloud config get-value core/account)

# For minikube
kubectl create clusterrolebinding cluster-admin-binding \
  --clusterrole=cluster-admin \
  --user=$USER
```

### Deploy Istio

```shell
kubectl apply -f ./third_party/istio-1.1.2/istio-crds.yaml
while [ $(kubectl get crd gateways.networking.istio.io -o jsonpath='{.status.conditions[?(@.type=="Established")].status}') != 'True' ]; do
  echo "Waiting on Istio CRDs"; sleep 1
done
kubectl apply -f ./third_party/istio-1.1.2/istio.yaml
```

Follow the
[instructions](https://www.knative.dev/docs/serving/gke-assigning-static-ip-address/)
if you need to set up static IP for Ingresses in the cluster.

### Deploy Knative Serving

This step includes building Knative Serving, creating and pushing developer
images and deploying them to your Kubernetes cluster.

First, edit [config-network.yaml](config/config-network.yaml) as instructed
within the file. If this file is edited and deployed after Knative Serving
installation, the changes in it will be effective only for newly created
revisions. Alternatively, if you are developing on GKE, you can skip the editing
and use the patching tool in `hack/dev-patch-config-gke.sh` after deploying
knative.

Next, run:

```shell
ko apply -f config/

# Optional steps

# Configure outbound network for GKE.
PROJECT_ID="my-gcp-project-id" ./hack/dev-patch-config-gke.sh my-k8s-cluster-name

# Run post-install job to setup nice XIP.IO domain name.  This only works
# if your Kubernetes LoadBalancer has an IP address.
ko delete -f config/post-install --ignore-not-found
ko apply -f config/post-install
```

The above step is equivalent to applying the `serving.yaml` for released
versions of Knative Serving.

You can see things running with:

```console
kubectl -n knative-serving get pods
NAME                          READY     STATUS    RESTARTS   AGE
activator-c8495dc9-z7xpz      2/2       Running   0          6d
autoscaler-66897845df-t5cwg   2/2       Running   0          6d
controller-699fb46bb5-xhlkg   1/1       Running   0          6d
webhook-76b87b8459-tzj6r      1/1       Running   0          6d
```

You can access the Knative Serving Controller's logs with:

```shell
kubectl -n knative-serving logs $(kubectl -n knative-serving get pods -l app=controller -o name)
```

If you're using a GCP project to host your Kubernetes cluster, it's good to
check the
[Discovery & load balancing](http://console.developers.google.com/kubernetes/discovery)
page to ensure that all services are up and running (and not blocked by a quota
issue, for example).

### Install logging and monitoring backends

Run:

```shell
kubectl apply -R -f config/monitoring/100-namespace.yaml \
    -f third_party/config/monitoring/logging/elasticsearch \
    -f config/monitoring/logging/elasticsearch \
    -f third_party/config/monitoring/metrics/prometheus \
    -f config/monitoring/metrics/prometheus \
    -f config/monitoring/tracing/zipkin
```

## Iterating

As you make changes to the code-base, there are two special cases to be aware
of:

- **If you change an input to generated code**, then you must run
  [`./hack/update-codegen.sh`](./hack/update-codegen.sh). Inputs include:

  - API type definitions in
    [pkg/apis/serving/v1alpha1/](./pkg/apis/serving/v1alpha1/.),
  - Types definitions annotated with `// +k8s:deepcopy-gen=true`.

- **If you change a package's deps** (including adding external dep), then you
  must run [`./hack/update-deps.sh`](./hack/update-deps.sh).

These are both idempotent, and we expect that running these at `HEAD` to have no
diffs. Code generation and dependencies are automatically checked to produce no
diffs for each pull request.

update-deps.sh runs "dep ensure" command. In some cases, if newer dependencies
are required, you need to run "dep ensure -update package-name" manually.

Once the codegen and dependency information is correct, redeploying the
controller is simply:

```shell
ko apply -f config/controller.yaml
```

Or you can [clean it up completely](./DEVELOPMENT.md#clean-up) and
[completely redeploy `Knative Serving`](./DEVELOPMENT.md#starting-knative-serving).

## Clean up

You can delete all of the service components with:

```shell
ko delete --ignore-not-found=true \
  -f config/monitoring/100-namespace.yaml \
  -f config/ \
  -f ./third_party/config/build/release.yaml \
  -f ./third_party/istio-1.1.2/istio.yaml \
  -f ./third_party/istio-1.1.2/istio-crds.yaml
```

## Telemetry

To access Telemetry see:

- [Accessing Metrics](https://www.knative.dev/docs/serving/accessing-metrics/)
- [Accessing Logs](https://www.knative.dev/docs/serving/accessing-logs/)
- [Accessing Traces](https://www.knative.dev/docs/serving/accessing-traces/)
