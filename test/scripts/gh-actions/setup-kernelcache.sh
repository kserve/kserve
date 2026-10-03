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

# Prepares the cluster for the KernelCache E2E tests (kserve/kserve#6337).
#
# Run this after setup-kserve.sh, which already installed KServe with
# ENABLE_LOCALMODEL=true (CRDs, webhook, controllers and the
# kserve-kernelcachenode-agent DaemonSet). This script only adds the
# KernelCache-specific setup. Every step is idempotent, and every readiness
# check uses kubectl wait or kubectl rollout status rather than a fixed sleep.
#
# Usage:
#   KERNELCACHE_REGISTRY_ENDPOINT=127.0.0.1:5000 ./setup-kernelcache.sh
#
# Environment variables:
#   KERNELCACHE_REGISTRY_ENDPOINT  Required. OCI registry host:port used by
#                                   capture and prefetch jobs. The KernelCache
#                                   config validation rejects a value that is
#                                   empty or contains "/", and the E2E setup
#                                   requires an explicit host:port form.
#   KSERVE_MCV_IMAGE               Optional. Defaults to the in-tree default
#                                   kserve/kserve-mcv:latest-minimal.

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]:-$0}")" &>/dev/null && pwd 2>/dev/null)"
source "${SCRIPT_DIR}/../../../hack/setup/common.sh"

KSERVE_NAMESPACE="${KSERVE_NAMESPACE:-kserve}"
export KSERVE_NAMESPACE

KERNELCACHE_REGISTRY_ENDPOINT="${KERNELCACHE_REGISTRY_ENDPOINT:-}"
KSERVE_MCV_IMAGE="${KSERVE_MCV_IMAGE:-kserve/kserve-mcv:latest-minimal}"

NODE_GROUP_NAME="kc-test-group"
NODE_GROUP_LABEL="kernelcache.example.com/group=workers"
JOBS_NAMESPACE="kserve-kernelcache-jobs"
WAIT_TIMEOUT="${WAIT_TIMEOUT:-300s}"

check_cli_exist kubectl jq

# pkg/apis/serving/v1beta1/configmap.go rejects an endpoint that is empty or
# that contains "/", a space, a tab or a newline.
if [[ -z "$KERNELCACHE_REGISTRY_ENDPOINT" ]]; then
    log_error "KERNELCACHE_REGISTRY_ENDPOINT is required (host:port, e.g. 127.0.0.1:5000)"
    exit 1
elif [[ "$KERNELCACHE_REGISTRY_ENDPOINT" == *"://"* || "$KERNELCACHE_REGISTRY_ENDPOINT" == *"/"* ]]; then
    log_error "KERNELCACHE_REGISTRY_ENDPOINT must be host:port without a scheme or path, got '${KERNELCACHE_REGISTRY_ENDPOINT}'"
    exit 1
elif [[ ! "$KERNELCACHE_REGISTRY_ENDPOINT" =~ ^[^:[:space:]]+:[0-9]+$ ]]; then
    log_error "KERNELCACHE_REGISTRY_ENDPOINT must be host:port, got '${KERNELCACHE_REGISTRY_ENDPOINT}'"
    exit 1
fi

log_info "Configuring KernelCache for the e2e tests"
log_info "  registry endpoint: ${KERNELCACHE_REGISTRY_ENDPOINT}"
log_info "  mcv image        : ${KSERVE_MCV_IMAGE}"
log_info "  node group       : ${NODE_GROUP_NAME} (${NODE_GROUP_LABEL})"
log_info "  jobs namespace   : ${JOBS_NAMESPACE}"

# Label the worker nodes that may host prepared artifacts. #6337 targets a
# multi-node cluster, so a cluster without worker nodes is a real failure
# rather than something to work around.
if [[ -z "$(kubectl get nodes -l '!node-role.kubernetes.io/control-plane' -o name)" ]]; then
    log_error "No worker node matches '!node-role.kubernetes.io/control-plane'; a worker node is required to host KernelCache artifacts"
    exit 1
fi
log_info "Labeling worker nodes with ${NODE_GROUP_LABEL} ..."
kubectl label nodes -l '!node-role.kubernetes.io/control-plane' "$NODE_GROUP_LABEL" --overwrite

# kernelcache.defaultNodeGroup names a KernelCacheNodeGroup resource, it is not
# a node label: the reconciler does a cluster-scoped Get on that name and reports
# "KernelCacheNodeGroup %q was not found" when it is missing. The node group
# selects its nodes through spec.nodeSelector, so it is wired to the label above.
log_info "Creating KernelCacheNodeGroup ${NODE_GROUP_NAME} ..."
kubectl apply -f - <<EOF
apiVersion: serving.kserve.io/v1alpha1
kind: KernelCacheNodeGroup
metadata:
  name: ${NODE_GROUP_NAME}
spec:
  nodeSelector:
    ${NODE_GROUP_LABEL%%=*}: ${NODE_GROUP_LABEL#*=}
EOF

create_or_skip_namespace "$JOBS_NAMESPACE"

# Update only the "kernelcache" key of inferenceservice-config. update_isvc_config
# builds the value with jq and leaves the rest of the ConfigMap data untouched.
# The KernelCache controller watches the ConfigMap, so no restart is needed.
log_info "Updating the kernelcache section of inferenceservice-config ..."
update_isvc_config \
    "kernelcache.enabled=true" \
    "kernelcache.defaultSidecarInjection=true" \
    "kernelcache.defaultMountType=oci" \
    "kernelcache.defaultNodeGroup=${NODE_GROUP_NAME}" \
    "kernelcache.jobNamespace=${JOBS_NAMESPACE}" \
    "kernelcache.mcvImage=${KSERVE_MCV_IMAGE}" \
    "kernelcache.prefetchImage=registry.access.redhat.com/ubi9/ubi-minimal:latest" \
    "kernelcache.registry.endpoint=${KERNELCACHE_REGISTRY_ENDPOINT}" \
    "kernelcache.registry.insecure=true" \
    "kernelcache.registry.auth.type=none" \
    "kernelcache.artifactSecurity.mode=cert" \
    "kernelcache.artifactSecurity.failurePolicy=reject" \
    "kernelcache.artifactSecurity.cert.signingProfileRef=kernelcache-signer" \
    "kernelcache.artifactSecurity.cert.trustBundle=kserve/kernelcache-root-ca" \
    "kernelcache.artifactSecurity.cert.subjectRegexp=spiffe://kserve/kernelcache-signer" \
    "kernelcache.abandonedCapturePolicy=retain" \
    "kernelcache.jobTTLSecondsAfterFinished=600" \
    "kernelcache.mcvCaptureReadinessTimeoutSeconds=600" \
    "kernelcache.reconcileIntervalSeconds=300"

# Read the ConfigMap back and fail if the kernelcache section is not exactly the
# expected configuration. The diff output shows precisely which field is wrong.
log_info "Verifying the kernelcache section of inferenceservice-config ..."
ACTUAL_KERNELCACHE="$(kubectl get configmap inferenceservice-config -n "$KSERVE_NAMESPACE" -o json |
    jq -er '.data.kernelcache | fromjson')"
EXPECTED_KERNELCACHE="$(jq -n --arg image "$KSERVE_MCV_IMAGE" --arg endpoint "$KERNELCACHE_REGISTRY_ENDPOINT" '{
    "enabled": true,
    "defaultSidecarInjection": true,
    "defaultMountType": "oci",
    "defaultNodeGroup": "kc-test-group",
    "jobNamespace": "kserve-kernelcache-jobs",
    "mcvImage": $image,
    "prefetchImage": "registry.access.redhat.com/ubi9/ubi-minimal:latest",
    "registry": {
        "endpoint": $endpoint,
        "insecure": true,
        "auth": { "type": "none" }
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
}')"
if ! diff <(jq -S . <<< "$ACTUAL_KERNELCACHE") <(jq -S . <<< "$EXPECTED_KERNELCACHE"); then
    log_error "inferenceservice-config kernelcache does not match the expected configuration"
    exit 1
fi

log_info "Applying the KernelCache certificates ..."
kubectl apply -f "${REPO_ROOT}/config/certmanager/localmodel/kc-certificate.yaml"
kubectl wait --for=condition=Ready \
    certificate/kernelcache-root-ca \
    certificate/kernelcache-signer \
    -n "$KSERVE_NAMESPACE" \
    --timeout="$WAIT_TIMEOUT"

log_info "Waiting for the KServe controllers and the KernelCacheNode agent ..."
kubectl rollout status deployment/kserve-controller-manager -n "$KSERVE_NAMESPACE" --timeout="$WAIT_TIMEOUT"
kubectl rollout status deployment/kserve-localmodel-controller-manager -n "$KSERVE_NAMESPACE" --timeout="$WAIT_TIMEOUT"
kubectl rollout status daemonset/kserve-kernelcachenode-agent -n "$KSERVE_NAMESPACE" --timeout="$WAIT_TIMEOUT"

log_success "KernelCache e2e setup complete."
