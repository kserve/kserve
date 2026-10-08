#!/bin/bash

# Copyright 2026 The KServe Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Configure the KernelCache feature on a cluster that already has KServe
# installed via setup-kserve.sh. Must be called after setup-kserve.sh.
#
# Required environment variables:
#   KERNELCACHE_REGISTRY_ENDPOINT  OCI registry the MCV sidecar pushes captured
#                                  artifacts to (e.g. "localhost:5000" for a
#                                  Minikube registry addon, or the in-cluster
#                                  registry address).
#
# Optional environment variables (all have defaults):
#   KERNELCACHE_NODE_GROUP    KernelCacheNodeGroup name (default: kc-test-group)
#   KERNELCACHE_NODE_LABEL    Node label selector applied to worker nodes
#                             (default: kernelcache.example.com/group=workers)
#   KERNELCACHE_JOBS_NS       Namespace for KC prefetch jobs (default: ${KERNELCACHE_JOBS_NS})
#   KERNELCACHE_REGISTRY_INSECURE  Set to "true" to allow plain-HTTP registry (default: true)

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]:-$0}")" &>/dev/null && pwd 2>/dev/null)"
source "${SCRIPT_DIR}/../../../hack/setup/common.sh"
source "${REPO_ROOT}/kserve-images.sh"

# ── Configuration ──────────────────────────────────────────────────────────
KERNELCACHE_NODE_GROUP="${KERNELCACHE_NODE_GROUP:-kc-test-group}"
KERNELCACHE_NODE_LABEL_KEY="${KERNELCACHE_NODE_LABEL_KEY:-nvidia.com/gpu.present}"
KERNELCACHE_NODE_LABEL_VALUE="${KERNELCACHE_NODE_LABEL_VALUE:-true}"
KERNELCACHE_JOBS_NS="kserve-kernelcache-jobs"
KERNELCACHE_REGISTRY_ENDPOINT="image-registry.openshift-image-registry.svc:5000"
if [ -z "$KO_DOCKER_REPO" ]; then
  exit 1
fi

export KO_DEFAULTPLATFORMS=linux/amd64
export TAG=${TAG:-test-gkm}
export MCV_IMAGE="${KO_DOCKER_REPO}/${MCV_IMG}:${TAG}-minimal"

# make docker-build docker-push
# make docker-build-kernelcachenode-agent docker-push-kernelcachenode-agent
# make docker-build-localmodel docker-push-localmodel
# make docker-build-mcv-minimal docker-push-mcv-minimal

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
    "endpoint": "image-registry.openshift-image-registry.svc:5000",
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