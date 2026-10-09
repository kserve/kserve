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
#   KERNELCACHE_JOBS_NS       Namespace for KC prefetch jobs (default: kserve-kernelcache-jobs)
#   KERNELCACHE_REGISTRY_INSECURE  Set to "true" to allow plain-HTTP registry (default: true)

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]:-$0}")" &>/dev/null && pwd 2>/dev/null)"
source "${SCRIPT_DIR}/../../../hack/setup/common.sh"
source "${REPO_ROOT}/kserve-images.sh"

# ── Configuration ──────────────────────────────────────────────────────────
KERNELCACHE_NODE_GROUP="${KERNELCACHE_NODE_GROUP:-kc-test-group}"
KERNELCACHE_NODE_LABEL_KEY="${KERNELCACHE_NODE_LABEL_KEY:-kernelcache.example.com/group}"
KERNELCACHE_NODE_LABEL_VALUE="${KERNELCACHE_NODE_LABEL_VALUE:-workers}"
KERNELCACHE_JOBS_NS="${KERNELCACHE_JOBS_NS:-kserve-kernelcache-jobs}"
KERNELCACHE_REGISTRY_INSECURE="${KERNELCACHE_REGISTRY_INSECURE:-true}"

# common.sh consumes KSERVE_NAMESPACE but never assigns it, so under
# `set -o nounset` every `-n "${KSERVE_NAMESPACE}"` below would abort with
# "KSERVE_NAMESPACE: unbound variable" unless the caller exports it.
# setup-kserve.sh does not, so default and export it here.
KSERVE_NAMESPACE="${KSERVE_NAMESPACE:-kserve}"
export KSERVE_NAMESPACE

if [[ -z "${KERNELCACHE_REGISTRY_ENDPOINT:-}" ]]; then
  log_error "KERNELCACHE_REGISTRY_ENDPOINT must be set (e.g. 'localhost:5000')"
  exit 1
fi

MCV_IMAGE="${KO_DOCKER_REPO}/${MCV_IMG}:${TAG}-minimal"

log_info "KernelCache setup"
log_info "  node group  : ${KERNELCACHE_NODE_GROUP}"
log_info "  node label  : ${KERNELCACHE_NODE_LABEL_KEY}=${KERNELCACHE_NODE_LABEL_VALUE}"
log_info "  jobs ns     : ${KERNELCACHE_JOBS_NS}"
log_info "  registry    : ${KERNELCACHE_REGISTRY_ENDPOINT} (insecure=${KERNELCACHE_REGISTRY_INSECURE})"
log_info "  mcv image   : ${MCV_IMAGE}"

# ── Step 1: Label worker nodes ─────────────────────────────────────────────
log_info "Labeling worker nodes with ${KERNELCACHE_NODE_LABEL_KEY}=${KERNELCACHE_NODE_LABEL_VALUE} ..."
kubectl label nodes \
  -l '!node-role.kubernetes.io/control-plane' \
  "${KERNELCACHE_NODE_LABEL_KEY}=${KERNELCACHE_NODE_LABEL_VALUE}" \
  --overwrite

# ── Step 2: Create the KernelCacheNodeGroup ─────────────────────────────────
# kernelcache.defaultNodeGroup set in step 4 names a KernelCacheNodeGroup
# resource; it is not a node label. The reconciler does a cluster-scoped Get on
# that name and reports "KernelCacheNodeGroup %q was not found" when it is
# missing, so the group has to exist before any KernelCache is reconciled. Its
# spec.nodeSelector is wired to the label applied in step 1. kubectl apply keeps
# this idempotent across repeated runs.
log_info "Creating KernelCacheNodeGroup '${KERNELCACHE_NODE_GROUP}' ..."
kubectl apply -f - <<EOF
apiVersion: serving.kserve.io/v1alpha1
kind: KernelCacheNodeGroup
metadata:
  name: ${KERNELCACHE_NODE_GROUP}
spec:
  nodeSelector:
    ${KERNELCACHE_NODE_LABEL_KEY}: ${KERNELCACHE_NODE_LABEL_VALUE}
EOF

# ── Step 3: Create KernelCache jobs namespace ──────────────────────────────
log_info "Creating KernelCache jobs namespace '${KERNELCACHE_JOBS_NS}' ..."
create_or_skip_namespace "${KERNELCACHE_JOBS_NS}"

# ── Step 4: Patch inferenceservice-config with KernelCache settings ────────
log_info "Patching inferenceservice-config with KernelCache settings ..."
KERNELCACHE_CONFIG=$(cat <<EOF
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
    "insecure": ${KERNELCACHE_REGISTRY_INSECURE},
    "auth": {
      "type": "none"
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
)

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

# ── Step 5: Verify certificates have been issued ────────────────────────────
log_info "Verify KernelCache certificates have been issued ..."
kubectl wait \
  --for=condition=Ready \
  certificate/kernelcache-root-ca \
  certificate/kernelcache-signer \
  -n "${KSERVE_NAMESPACE}" \
  --timeout=120s

# ── Step 6: Restart localmodel controller and wait for rollout ────────────
# The localmodel controller's webhook server needs a TLS certificate from
# cert-manager (localmodel-webhook-server-cert). That cert is created during
# initial cluster setup, but cert-manager may not have issued it before the
# controller pod first started — causing a crash loop. Now that the KernelCache
# certs are ready (step 4), cert-manager has been running long enough to have
# issued all TLS certs. Restart the controller so it gets a fresh start with
# all secrets already mounted.
log_info "Restarting kserve-localmodel-controller-manager to pick up TLS certificates ..."
kubectl rollout restart deployment/kserve-localmodel-controller-manager \
  -n "${KSERVE_NAMESPACE}"

log_info "Waiting for kserve-localmodel-controller-manager rollout ..."
kubectl rollout status deployment/kserve-localmodel-controller-manager \
  -n "${KSERVE_NAMESPACE}" --timeout=300s

log_info "Waiting for kserve-controller-manager rollout ..."
kubectl rollout status deployment/kserve-controller-manager \
  -n "${KSERVE_NAMESPACE}" --timeout=300s

# ── Step 7: Wait for KernelCacheNode agent DaemonSet ──────────────────────
log_info "Waiting for kserve-kernelcachenode-agent DaemonSet ..."
kubectl rollout status daemonset/kserve-kernelcachenode-agent \
  -n "${KSERVE_NAMESPACE}" --timeout=300s

log_success "KernelCache setup complete."
