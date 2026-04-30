# OLM Bundle

This document describes how to generate, validate, test, and release the KServe
OLM (Operator Lifecycle Manager) bundle for community operators.

## Bundle Structure

```
bundle/
  manifests/         # CSV and CRD manifests
  metadata/          # Bundle annotations
  tests/scorecard/   # Scorecard test configuration
bundle.Dockerfile    # Bundle image Dockerfile
```

The bundle is an all-in-one package that includes all three KServe controllers:

- **kserve-controller-manager** — Core inference serving (InferenceService, ServingRuntime, InferenceGraph, etc.)
- **llmisvc-controller-manager** — LLM inference (LLMInferenceService, LLMInferenceServiceConfig)
- **kserve-localmodel-controller-manager** — Local model caching (LocalModelCache, LocalModelNode, etc.)

### Key Files

| File | Purpose |
|------|---------|
| `config/manifests/bases/kserve.clusterserviceversion.yaml` | Base CSV template (version, descriptions, alm-examples, CRD ownership) |
| `config/manifests/kustomization.yaml` | Kustomize composition that feeds into `operator-sdk generate bundle` |
| `config/manifests/patches/` | Kustomize patches applied during bundle generation |
| `Makefile` | `BUNDLE_VERSION` variable and bundle targets |
| `bundle/manifests/kserve.clusterserviceversion.yaml` | Generated CSV (do not edit directly) |

## Kustomize Patches

The bundle generation uses kustomize patches in `config/manifests/patches/` to
adapt the upstream manifests for OLM without modifying the source files directly.
This ensures upstream changes flow through automatically.

### Webhook Name Shortening (`patches/shorten-webhook-names.yaml`)

OLM maps webhook `generateName` values to Kubernetes label values, which are
limited to 63 characters. The `LLMInferenceServiceConfig` webhook names
(`llminferenceserviceconfig.kserve-webhook-server.v1alpha{1,2}.validator`) are
66 characters and exceed this limit. This patch shortens them to 54 characters
using a JSON 6902 `replace` operation that only changes the `name` field,
preserving all other webhook configuration:

```yaml
- op: replace
  path: /webhooks/0/name
  value: llmisvcconfig.kserve-webhook-server.v1alpha1.validator
- op: replace
  path: /webhooks/1/name
  value: llmisvcconfig.kserve-webhook-server.v1alpha2.validator
```

This issue does not affect Helm or plain `kubectl apply` deployments because
they do not enforce label-length limits on webhook names. It is specific to OLM.

### Default Deployment Mode (`patches/deploy-mode-standard.yaml`)

Overrides the default deployment mode from `Serverless` (requires Knative) to
`RawDeployment` for community OLM deployments where Knative is not expected:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: inferenceservice-config
data:
  deploy: |-
    {
        "defaultDeploymentMode": "RawDeployment"
    }
```

### Webhook Service Port (`patches/webhook-service-port.yaml`)

OLM routes webhook traffic to a service named `{deploymentName}-service`. For
`kserve-controller-manager` and `llmisvc-controller-manager`, the bundle already
ships services with these names that only expose the metrics port (8443). Without
this patch, OLM uses those services as-is and webhook calls fail with
`no service port 443 found`.

This patch adds port 443 (targeting the webhook-server container port 9443) to
both services:

```yaml
- op: add
  path: /spec/ports/-
  value:
    name: webhook-server
    port: 443
    targetPort: webhook-server
    protocol: TCP
```

The `kserve-localmodel-controller-manager` does not need this patch because it
does not have a pre-existing `*-controller-manager-service` in the bundle — OLM
auto-creates one with port 443.

## Generating the Bundle

```bash
make bundle
```

This runs `kustomize build config/manifests | operator-sdk generate bundle` with
the version, channels, and package configured in the Makefile, then restores the
`alm-examples` annotation (which operator-sdk clears during generation) and
validates the bundle.

### Makefile Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `BUNDLE_VERSION` | `0.17.0` | Operator version stamped into the CSV |
| `BUNDLE_CHANNELS` | `stable,alpha` | OLM channels |
| `BUNDLE_DEFAULT_CHANNEL` | `stable` | Default channel |
| `BUNDLE_PACKAGE` | `kserve` | OLM package name |
| `BUNDLE_IMG` | `$(KO_DOCKER_REPO)/kserve-operator-bundle:v$(BUNDLE_VERSION)` | Bundle image reference |

## Updating the Operator Version

When releasing a new version, **two things must be updated**:

### 1. Update `BUNDLE_VERSION`

In the `Makefile`, update `BUNDLE_VERSION` to the new release version:

```makefile
BUNDLE_VERSION ?= 0.18.0
```

This version is passed to `operator-sdk generate bundle --version` and gets
stamped into the generated CSV as `spec.version` and into the CSV name
(`kserve.v0.18.0`). The base CSV uses `0.0.0` as a placeholder that gets
overridden during generation.

### 2. Add `spec.replaces` to the Base CSV

In `config/manifests/bases/kserve.clusterserviceversion.yaml`, add a `replaces`
field under `spec` pointing to the **previous** version. This tells OLM how to
upgrade from one version to the next:

```yaml
spec:
  replaces: kserve.v0.17.0
  # ...existing fields...
```

The `replaces` field is **required** for all versions after the initial release.
Without it, OLM cannot build the upgrade graph and existing installations will
not receive updates.

### Version Checklist

- [ ] Update `BUNDLE_VERSION` in `Makefile`
- [ ] Add/update `spec.replaces` in `config/manifests/bases/kserve.clusterserviceversion.yaml`
- [ ] Update `containerImage` annotation if the controller image tag changed
- [ ] Regenerate the bundle: `make bundle`
- [ ] Validate: `make bundle-validate`

## Validation

### Basic Validation

The `make bundle` target runs the operatorframework validation suite
automatically after generation. To run validation separately:

```bash
make bundle-validate
```

This runs two validation suites:

- `suite=operatorframework` — Checks CSV structure, required fields, and API annotations
- `name=community` — Checks community operator requirements

### Scorecard Tests

The bundle includes scorecard tests under `bundle/tests/scorecard/config.yaml`.
To run them against a live cluster:

```bash
operator-sdk scorecard ./bundle
```

This runs the following tests:

| Test | Suite | What it checks |
|------|-------|----------------|
| `basic-check-spec` | basic | Operator CR has a valid spec section |
| `olm-bundle-validation` | olm | Bundle format and structure |
| `olm-crds-have-validation` | olm | CRDs include OpenAPI validation |
| `olm-crds-have-resources` | olm | CSV lists resources for owned CRDs |
| `olm-spec-descriptors` | olm | CSV has spec descriptors for CRDs |
| `olm-status-descriptors` | olm | CSV has status descriptors for CRDs |

Scorecard tests require a running Kubernetes cluster with OLM installed. The
tests run as pods inside the cluster.

To run a specific suite:

```bash
operator-sdk scorecard ./bundle --selector=suite=basic
operator-sdk scorecard ./bundle --selector=suite=olm
```

## Building and Pushing the Bundle Image

```bash
make bundle-build
make bundle-push
```

## Local Testing with a Catalog

Before submitting to community operators, you can test the full OLM installation
flow locally by building a File-Based Catalog (FBC) that contains your bundle
image and deploying it to a cluster with OLM installed.

### Prerequisites

- [Minikube](https://minikube.sigs.k8s.io/docs/start/) installed
- [`operator-sdk`](https://sdk.operatorframework.io/docs/installation/) CLI
- [`opm`](https://github.com/operator-framework/operator-registry/releases) CLI - https://github.com/operator-framework/operator-registry/releases
  (must match the OLM version on your cluster)

#### Setting Up a Minikube Cluster with a Registry

Create a Minikube cluster with the `--insecure-registry` flag to allow the
built-in registry addon to work without TLS:

```bash
minikube start --driver=<docker|podman|qemu2> --cpus=4 --memory=8192 \
  --insecure-registry="10.0.0.0/24"
```

Enable the registry addon. This deploys an in-cluster registry service in the
`kube-system` namespace:

```bash
minikube addons enable registry
```

Forward the registry port to `localhost` so you can push images from the host.
The local port is assigned automatically (especially important with the podman
driver, which does not allow choosing a specific local port):

```bash
# Run in a separate terminal — this must stay running
kubectl port-forward -n kube-system svc/registry :80 > /tmp/registry-pf.log 2>&1 &
sleep 2

# Capture the assigned local port
export LOCAL_PORT=$(grep -oE '127.0.0.1:[0-9]+' /tmp/registry-pf.log | head -1 | cut -d: -f2)
echo "Registry forwarded to localhost:$LOCAL_PORT"
```

Get the in-cluster registry IP (needed for catalog image references):

```bash
export REGISTRY_IP=$(kubectl -n kube-system get svc registry \
  -o jsonpath='{.spec.clusterIP}')
echo "Registry in-cluster IP: $REGISTRY_IP"
```

Use `localhost:$LOCAL_PORT` when building and pushing images from the host, and
`${REGISTRY_IP}:80` for in-cluster image references (CatalogSource, FBC
catalog entries).

Enable Dashboard (optional):

```bash
minikube dashboard
```

#### Installing OLM on the Cluster

```bash
# it requires to be already logged in on the cluster
operator-sdk olm install
```

Verify OLM is running:

```bash
operator-sdk olm status
```

### Step 1 — Build and Push the Bundle Image

Build the bundle image and push it to a registry the cluster can access:

```bash
# Using the Minikube registry addon (port-forwarded to localhost:$LOCAL_PORT)
export BUNDLE_IMG=localhost:${LOCAL_PORT}/kserve-operator-bundle:v0.17.0

make bundle-build BUNDLE_IMG=$BUNDLE_IMG

# TLS_VERIFY=false is required because the Minikube registry addon is HTTP-only
make bundle-push BUNDLE_IMG=$BUNDLE_IMG TLS_VERIFY=false

# Podman on macOS: podman push runs inside the podman machine VM and cannot
# reach the host's port-forwarded registry. Use minikube image load instead:
#   minikube image load $BUNDLE_IMG
```

### Step 2 — Build and Push the Catalog Image

Use `opm` to render the bundle into a File-Based Catalog and build a catalog
image. The catalog must contain three FBC entries: `olm.bundle` (from `opm
render`), `olm.package`, and `olm.channel`.

```bash
export CATALOG_IMG=localhost:${LOCAL_PORT}/kserve-operator-catalog:v0.17.0

# Render the bundle into FBC format.
# From a registry (use --use-http for HTTP-only registries):
#   opm render $BUNDLE_IMG --output=yaml --use-http > /tmp/catalog-bundle.yaml
# From the local bundle directory (no registry needed):
opm render ./bundle --output=yaml > /tmp/catalog-bundle.yaml

# Replace localhost references with the in-cluster registry IP and port
# (pods resolve the registry by its ClusterIP, not localhost).
# When rendered from a local directory the image field is empty — set it to the
# in-cluster registry reference so the catalog pod can pull the bundle.
sed "s|image: \"\"|image: ${REGISTRY_IP}:80/kserve-operator-bundle:v0.17.0|g; s|localhost:${LOCAL_PORT}|${REGISTRY_IP}:80|g" \
  /tmp/catalog-bundle.yaml > /tmp/catalog.yaml

# Append the package and channel entries
cat >> /tmp/catalog.yaml <<'EOF'
---
schema: olm.package
name: kserve
defaultChannel: stable
---
schema: olm.channel
name: stable
package: kserve
entries:
  - name: kserve.v0.17.0
EOF
```

Build the catalog image using a multi-stage Dockerfile that pre-builds the
cache (required for FBC catalogs):

```bash
cat > /tmp/catalog.Dockerfile <<'EOF'
FROM registry.redhat.io/openshift4/ose-operator-registry-rhel9:v4.18 AS builder
COPY catalog.yaml /configs/catalog.yaml
RUN ["/bin/opm", "serve", "/configs", "--cache-dir=/tmp/cache", "--cache-only"]

FROM registry.redhat.io/openshift4/ose-operator-registry-rhel9:v4.18
COPY --from=builder /configs /configs
COPY --from=builder /tmp/cache /tmp/cache
EXPOSE 50051
ENTRYPOINT ["/bin/opm"]
CMD ["serve", "/configs", "--cache-dir=/tmp/cache"]
EOF

${ENGINE:-docker} build -f /tmp/catalog.Dockerfile -t $CATALOG_IMG /tmp
${ENGINE:-docker} push --tls-verify=false $CATALOG_IMG

# Podman on macOS: podman push runs inside the podman machine VM and cannot
# reach the host's port-forwarded registry. Load images into minikube and
# push them to the in-cluster registry from inside the node:
#   minikube image load $BUNDLE_IMG
#   minikube image load $CATALOG_IMG
#   REGISTRY_IP=$(kubectl -n kube-system get svc registry -o jsonpath='{.spec.clusterIP}')
#   minikube ssh -- "docker tag $CATALOG_IMG ${REGISTRY_IP}:80/kserve-operator-catalog:v0.17.0 && \
#     docker push ${REGISTRY_IP}:80/kserve-operator-catalog:v0.17.0 && \
#     docker tag $BUNDLE_IMG ${REGISTRY_IP}:80/kserve-operator-bundle:v0.17.0 && \
#     docker push ${REGISTRY_IP}:80/kserve-operator-bundle:v0.17.0"
```

### Step 3 — Create the CatalogSource

Apply a `CatalogSource` that points to your catalog image. Use the in-cluster
registry IP, not `localhost`:

```bash
kubectl apply -f - <<EOF
apiVersion: operators.coreos.com/v1alpha1
kind: CatalogSource
metadata:
  name: kserve-catalog
  namespace: olm
spec:
  sourceType: grpc
  image: ${REGISTRY_IP}:80/kserve-operator-catalog:v0.17.0
  displayName: KServe (dev)
  updateStrategy:
    registryPoll:
      interval: 10m
EOF
```

Wait for the catalog pod to become ready:

```bash
kubectl -n olm get catalogsource kserve-catalog
kubectl -n olm get pods -l olm.catalogSource=kserve-catalog
```

The `READY` column of the CatalogSource should show `True` and the catalog pod
should be in `Running` state.

> **Note:** If the catalog pod shows `ImagePullBackOff`, verify that the
> `--insecure-registry` flag was set when the Minikube cluster was created and
> that the image reference uses the in-cluster registry ClusterIP (port 80),
> not `localhost`.

### Step 4 — Install the Operator

Create the `kserve` namespace with an `OperatorGroup` and a `Subscription` to
install the operator:

```bash
kubectl create namespace kserve

kubectl apply -f - <<EOF
apiVersion: operators.coreos.com/v1
kind: OperatorGroup
metadata:
  name: kserve-og
  namespace: kserve
spec: {}
---
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: kserve-subscription
  namespace: kserve
spec:
  channel: stable
  name: kserve
  source: kserve-catalog
  sourceNamespace: olm
  installPlanApproval: Automatic
EOF
```

Monitor the installation:

```bash
# Watch the CSV status
kubectl -n kserve get csv

# Check operator pods
kubectl -n kserve get pods

# check install plans
kubectl -n kserve get csv
```

The CSV should reach the `Succeeded` phase and the three controller pods
(kserve-controller-manager, llmisvc-controller-manager,
kserve-localmodel-controller-manager) should be running.

### Step 5 — Post-Install: Apply ClusterServingRuntimes and ClusterStorageContainer

OLM `registry+v1` bundles can only contain CRDs, CSVs, and standard Kubernetes
objects. Custom resource instances (ClusterServingRuntime, ClusterStorageContainer)
cannot be included in the bundle. They must be applied manually after the
operator is installed:

```bash
kubectl apply -k config/runtimes/
kubectl apply -k config/storagecontainers/
```

All 13 ClusterServingRuntime definitions and the default ClusterStorageContainer
are included as `alm-examples` in the CSV. Users installing from OperatorHub can
use these templates to create the resources from the UI.

### Step 6 — Verify and Clean Up

Verify the CRDs are installed:

```bash
kubectl get crd | grep kserve
```

Verify ClusterServingRuntimes and deploy mode:

```bash
kubectl get clusterservingruntimes
kubectl get cm inferenceservice-config -n kserve -o jsonpath='{.data.deploy}'
```

Test with an InferenceService:

```bash
kubectl create namespace kserve-test
kubectl apply -f - <<EOF
apiVersion: serving.kserve.io/v1beta1
kind: InferenceService
metadata:
  name: sklearn-iris
  namespace: kserve-test
spec:
  predictor:
    sklearn:
      storageUri: "gs://kfserving-examples/models/sklearn/1.0/model"
EOF

# Wait for Ready
kubectl get isvc sklearn-iris -n kserve-test -w
```

Once the InferenceService shows `READY: True`, port-forward to the predictor
service and run an inference request:

```bash
kubectl port-forward -n kserve-test svc/sklearn-iris-predictor 8080:80 &
sleep 2

# Check model readiness
curl -s http://localhost:8080/v1/models/sklearn-iris
# Expected: {"name":"sklearn-iris","ready":true}

# Send an inference request
curl -s http://localhost:8080/v1/models/sklearn-iris:predict \
  -H "Content-Type: application/json" \
  -d '{"instances": [[6.8, 2.8, 4.8, 1.4], [6.0, 3.4, 4.5, 1.6]]}'
# Expected: {"predictions":[1,1]}
```

To uninstall and clean up:

```bash
kubectl delete namespace kserve-test
kubectl delete clusterservingruntimes --all
kubectl delete clusterstoragecontainers --all
kubectl -n kserve delete subscription kserve-subscription
kubectl -n kserve delete csv kserve.v0.17.0
kubectl -n olm delete catalogsource kserve-catalog
```

To also remove OLM itself:

```bash
operator-sdk olm uninstall
```

To delete the entire Minikube cluster:

```bash
minikube delete
```

## Troubleshooting

### OLM Caching Issues

OLM copies the CSV to **all namespaces** when using AllNamespaces install mode.
When redeploying during testing, stale CSVs in other namespaces can prevent a
clean reinstall. To fully clean up:

```bash
# Delete CSV from all namespaces
for ns in $(kubectl get csv --all-namespaces -o jsonpath='{range .items[?(@.metadata.name=="kserve.v0.17.0")]}{.metadata.namespace}{"\n"}{end}'); do
  kubectl delete csv kserve.v0.17.0 -n "$ns"
done

# Delete all KServe CRDs
kubectl get crd -o name | grep -E 'kserve|inference\.networking' | xargs kubectl delete
```

Minikube's containerd also caches images by tag. When redeploying with the
same tag, the old image may be served from cache. Either use unique tags
(e.g., `v0.17.0-$(date +%s)`) or flush the containerd cache:

```bash
minikube ssh -- sudo crictl rmi --all
```

### Webhook Service Port 443 Not Found

If `kubectl apply` of ClusterServingRuntimes or InferenceService fails with:

```
no service port 443 found for service "kserve-controller-manager-service"
```

This means the `webhook-service-port.yaml` kustomize patch is not applied.
Regenerate the bundle with `make bundle` and verify the generated services in
`bundle/manifests/` include both port 8443 (metrics) and port 443
(webhook-server).

### CatalogSource Shows TRANSIENT_FAILURE

The catalog pod cannot pull the catalog image. Check:

1. The image reference uses the in-cluster registry ClusterIP (port 80), not
   `localhost`
2. Minikube was started with `--insecure-registry="10.0.0.0/24"` (this must be
   set at cluster creation time — it cannot be changed after)
3. The registry addon is enabled: `minikube addons list | grep registry`

## Adding or Updating CRDs

When adding new CRDs or new versions of existing CRDs:

1. Add the CRD entry to `spec.customresourcedefinitions.owned` in the base CSV
   with `description`, `displayName`, `kind`, `name`, and `version`
2. Add an `alm-examples` entry for the new CRD in the base CSV annotations
3. Regenerate: `make bundle`

## Updating alm-examples

The `alm-examples` annotation in the base CSV
(`config/manifests/bases/kserve.clusterserviceversion.yaml`) contains JSON
examples of all owned CRDs. These examples are displayed in the OperatorHub UI
and serve as templates for users creating resources.

The base CSV includes alm-examples for all API types including all 13
ClusterServingRuntime definitions. When adding or modifying runtimes in
`config/runtimes/`, update the corresponding alm-examples entry in the base CSV.

operator-sdk clears this annotation during `generate bundle`. The Makefile
restores it automatically using `yq` after generation. Any changes to
alm-examples should be made in the **base CSV**, not in the generated bundle.

## Submitting to Community Operators

After the bundle is generated and validated:

1. Fork https://github.com/k8s-operatorhub/community-operators (vanilla k8s) and https://github.com/redhat-openshift-ecosystem/community-operators-prod (OpenShift)
2. Create a directory `operators/kserve/<version>/` (e.g., `operators/kserve/0.17.0/`)
3. Copy the contents of `bundle/manifests/` and `bundle/metadata/` into that directory
4. Copy the `bundle.Dockerfile` into that directory
   5. be sure to verify if the `release-config.yaml` is present in the new version, if not, just copy from the previous one.
5. Submit a pull request following the [community operators contribution guide](https://k8s-operatorhub.github.io/community-operators/contributing-via-pr/)