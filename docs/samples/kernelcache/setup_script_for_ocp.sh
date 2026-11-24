#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]:-$0}")" &>/dev/null && pwd 2>/dev/null)"
source "${SCRIPT_DIR}/../../../hack/setup/common.sh"

if [ -z "${KO_DOCKER_REPO:-}" ]; then
  log_error "KO_DOCKER_REPO is required. Set it to the registry repository for the images, for example: export KO_DOCKER_REPO=image-registry.openshift-image-registry.svc:5000/<project>"
  exit 1
fi

if [ -z "${KERNELCACHE_NODE_LABEL_KEY:-}" ]; then
  log_error "KERNELCACHE_NODE_LABEL_KEY is required. Set it to the label key used to select KernelCache nodes, for example: export KERNELCACHE_NODE_LABEL_KEY=nvidia.com/gpu.present"
  exit 1
fi

if [ -z "${KERNELCACHE_NODE_LABEL_VALUE:-}" ]; then
  log_error "KERNELCACHE_NODE_LABEL_VALUE is required. Set it to the label value used to select KernelCache nodes, for example: export KERNELCACHE_NODE_LABEL_VALUE=true"
  exit 1
fi

source "${REPO_ROOT}/kserve-images.sh"

# ── Configuration ──────────────────────────────────────────────────────────
KERNELCACHE_NODE_GROUP="${KERNELCACHE_NODE_GROUP:-kc-test-group}"
KERNELCACHE_NODE_LABEL_KEY="${KERNELCACHE_NODE_LABEL_KEY}"
KERNELCACHE_NODE_LABEL_VALUE="${KERNELCACHE_NODE_LABEL_VALUE}"
KERNELCACHE_JOBS_NS="kserve-kernelcache-jobs"
KERNELCACHE_REGISTRY_ENDPOINT="image-registry.openshift-image-registry.svc:5000"

export KO_DEFAULTPLATFORMS=linux/amd64
export TAG=${TAG:-test-gkm}
export MCV_IMAGE="${KO_DOCKER_REPO}/${MCV_IMG}:${TAG}-minimal"

make docker-build docker-push
make docker-build-kernelcachenode-agent docker-push-kernelcachenode-agent
make docker-build-localmodel docker-push-localmodel
make docker-build-mcv-minimal docker-push-mcv-minimal

log_info "KernelCache setup"
log_info "  node group  : ${KERNELCACHE_NODE_GROUP}"
log_info "  node label  : ${KERNELCACHE_NODE_LABEL_KEY}=${KERNELCACHE_NODE_LABEL_VALUE}"
log_info "  jobs ns     : ${KERNELCACHE_JOBS_NS}"
log_info "  registry    : ${KERNELCACHE_REGISTRY_ENDPOINT}"
log_info "  mcv image   : ${MCV_IMAGE}"

log_info "Creating KernelCache jobs namespace '${KERNELCACHE_JOBS_NS}' ..."
kubectl get namespace ${KERNELCACHE_JOBS_NS} >/dev/null 2>&1 || \
  kubectl create namespace ${KERNELCACHE_JOBS_NS}

log_info "Creating ClusterRole and ClusterRoleBinding for KernelCache ..."
cat <<EOF | kubectl apply -f -
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: kserve-kernelcache-openshift-registry-binder
rules:
- apiGroups:
  - rbac.authorization.k8s.io
  resources:
  - clusterroles
  resourceNames:
  - system:image-builder
  - system:image-puller
  verbs:
  - bind
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: kserve-kernelcache-openshift-registry-binder
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: kserve-kernelcache-openshift-registry-binder
subjects:
- kind: ServiceAccount
  name: kserve-localmodel-controller-manager
  namespace: kserve
EOF

log_info "Patching clusterstoragecontainer default with XDG_CACHE_HOME and HUGGINGFACE_HUB_CACHE ..."
kubectl patch clusterstoragecontainer default --type='json' -p='[
  {
    "op": "add",
    "path": "/spec/container/env",
    "value": [
      {
        "name": "XDG_CACHE_HOME",
        "value": "/tmp/xdg_cache"
      },
      {
        "name": "HUGGINGFACE_HUB_CACHE",
        "value": "/tmp/hf_home/hub"
      }
    ]
  }
]'

log_info "Patching inferenceservice-config with KernelCache settings ..."
KERNELCACHE_CONFIG="$(cat <<EOF
{
  "enabled": true,
  "defaultSidecarInjection": true,
  "defaultMountType": "oci",
  "defaultNodeGroup": "${KERNELCACHE_NODE_GROUP}",
  "jobNamespace": "${KERNELCACHE_JOBS_NS}",
  "mcvImage": "${MCV_IMAGE}",
  "prefetchImage": "registry.access.redhat.com/ubi9/ubi-minimal:latest",
  "registry": {
    "endpoint": "${KERNELCACHE_REGISTRY_ENDPOINT}",
    "caConfigMapRef": {
      "name": "openshift-service-ca.crt",
      "key": "service-ca.crt"
    },
    "auth": {
      "type": "serviceAccountToken",
      "tokenTTLSeconds": 600,
      "pushRoleRef": {
        "kind": "ClusterRole",
        "name": "system:image-builder"
      },
      "pullRoleRef": {
        "kind": "ClusterRole",
        "name": "system:image-puller"
      }
    }
  },
  "artifactSecurity": {
    "mode": "cert",
    "failurePolicy": "reject",
    "cert": {
      "signingProfileRef": "kernelcache-signer",
      "trustBundle": "kserve/kernelcache-root-ca",
      "subjectRegexp": "spiffe://kserve/kernelcache-signer"
    }
  },
  "abandonedCapturePolicy": "retain",
  "jobTTLSecondsAfterFinished": 600,
  "mcvCaptureReadinessTimeoutSeconds": 600,
  "reconcileIntervalSeconds": 300
}
EOF
)"$'\n'

PATCH=$(jq -n \
  --arg config "${KERNELCACHE_CONFIG}" \
  '[{
    "op": "replace",
    "path": "/data/kernelcache",
    "value": $config
  }]'
)

kubectl patch configmap inferenceservice-config \
  -n "${KSERVE_NAMESPACE}" \
  --type=json \
  -p "${PATCH}"


log_info "Patching kserve-controller-manager Deployment"
kubectl patch deployment -n kserve kserve-controller-manager \
-p "{\"spec\":{\"template\":{\"spec\":{\"containers\":[{\"name\":\"manager\",\"image\":\"${KO_DOCKER_REPO}/kserve-controller:${TAG}\"}]}}}}"

log_info "Patching kserve-localmodel-controller-manager Deployment"
kubectl patch deployment -n kserve kserve-localmodel-controller-manager \
-p "{\"spec\":{\"template\":{\"spec\":{\"containers\":[{\"name\":\"manager\",\"image\":\"${KO_DOCKER_REPO}/kserve-localmodel-controller:${TAG}\"}]}}}}"

log_info "Patching kserve-kernelcachenode-agent DaemonSet"
kubectl patch daemonset -n kserve kserve-kernelcachenode-agent \
-p "{\"spec\":{\"template\":{\"spec\":{\"containers\":[{\"name\":\"manager\",\"image\":\"${KO_DOCKER_REPO}/kserve-kernelcachenode-agent:${TAG}\",\"imagePullPolicy\":\"IfNotPresent\"}]}}}}"

kubectl rollout restart deployment/kserve-localmodel-controller-manager \
  -n "${KSERVE_NAMESPACE}"

log_info "Waiting for kserve-localmodel-controller-manager rollout ..."
kubectl rollout status deployment/kserve-localmodel-controller-manager \
  -n "${KSERVE_NAMESPACE}" --timeout=300s

log_info "Waiting for kserve-controller-manager rollout ..."
kubectl rollout status deployment/kserve-controller-manager \
  -n "${KSERVE_NAMESPACE}" --timeout=300s


log_info "Waiting for kserve-kernelcachenode-agent DaemonSet ..."
kubectl rollout status daemonset/kserve-kernelcachenode-agent \
  -n "${KSERVE_NAMESPACE}" --timeout=300s

log_info "Creating KernelCacheNodeGroup for GPU nodes ..."
cat <<EOF | kubectl apply -f -
apiVersion: serving.kserve.io/v1alpha1
kind: KernelCacheNodeGroup
metadata:
  name: ${KERNELCACHE_NODE_GROUP}
spec:
  nodeSelector:
    ${KERNELCACHE_NODE_LABEL_KEY}: "${KERNELCACHE_NODE_LABEL_VALUE}"
EOF


log_success "KernelCache setup complete."
