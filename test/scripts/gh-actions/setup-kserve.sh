#!/bin/bash

# Copyright 2022 The KServe Authors.
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

# The script will install KServe dependencies in the GH Actions environment.
# (Istio, Knative, cert-manager, kustomize, yq)
# Usage: setup-kserve.sh $DEPLOYMENT_MODE $NETWORK_LAYER

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]:-$0}")" &>/dev/null && pwd 2>/dev/null)"
source "${SCRIPT_DIR}/../../../hack/setup/common.sh"

export DEPLOYMENT_MODE="${1:-Knative}"
export NETWORK_LAYER="${2:-istio}"
export GATEWAY_NETWORK_LAYER="false"
export ENABLE_LLMISVC="${ENABLE_LLMISVC:-false}"
export ENABLE_KSERVE_WITH_LLMISVC="${ENABLE_KSERVE_WITH_LLMISVC:-false}"
export INSTALL_METHOD="${INSTALL_METHOD:-kustomize}"

# Extract gateway class name from NETWORK_LAYER (e.g., "envoy-gatewayapi" -> "envoy")
# If NETWORK_LAYER contains "-", extract the first part; otherwise, use "false"
if [[ $NETWORK_LAYER == *"-gatewayapi"* ]]; then
  export GATEWAY_NETWORK_LAYER="${NETWORK_LAYER%%-*}"
fi

echo "Installing KServe using ${INSTALL_METHOD^}..."

echo "Creating e2e test namespaces ..."
E2E_NS="${KSERVE_TEST_NAMESPACE:-kserve-ci-e2e-test}"
kubectl get namespace "$E2E_NS" || kubectl create namespace "$E2E_NS"
kubectl label namespace "$E2E_NS" kserve.io/e2e-test=true --overwrite 2>/dev/null || true

E2E_WORKERS="${E2E_WORKER_COUNT:-0}"
if [[ "$E2E_WORKERS" -gt 0 ]]; then
  for i in $(seq 0 $((E2E_WORKERS - 1))); do
    WORKER_NS="${E2E_NS}-gw${i}"
    kubectl get namespace "$WORKER_NS" 2>/dev/null || kubectl create namespace "$WORKER_NS"
    kubectl label namespace "$WORKER_NS" kserve.io/e2e-test=true --overwrite 2>/dev/null || true
  done
fi

echo "Installing KServe Python SDK ..."
pushd python/kserve >/dev/null
    uv sync --active --group test
popd


if [[ $ENABLE_LLMISVC == "false" || $ENABLE_KSERVE_WITH_LLMISVC == "true" ]]; then
  if [[ $INSTALL_METHOD == "helm" ]]; then
    export KSERVE_EXTRA_ARGS="--set kserve.controller.imagePullPolicy=IfNotPresent" 
    export LOCALMODEL_EXTRA_ARGS="--set kserve.localmodel.controller.imagePullPolicy=IfNotPresent --set kserve.localmodelnode.controller.imagePullPolicy=IfNotPresent" 
    export ENABLE_LOCALMODEL=true
    if [[ $ENABLE_LLMISVC == "true" ]]; then
      export LLMISVC_EXTRA_ARGS="--set kserve.llmisvc.controller.imagePullPolicy=IfNotPresent"
    fi
    export SET_KSERVE_VERSION=${TAG}
    export USE_LOCAL_CHARTS=true
    export INSTALL_RUNTIMES=true
    ${REPO_ROOT}/hack/setup/infra/manage.kserve-helm.sh
    kustomize build config/overlays/test/s3-local-backend | kubectl apply --server-side --force-conflicts -f -
  else
    export SET_KSERVE_VERSION=${TAG}
    export ENABLE_LOCALMODEL=true
    export KSERVE_OVERLAY_DIR=test
    export INSTALL_RUNTIMES=false
    if [[ $ENABLE_LLMISVC == "true" ]]; then
      if [[ $ENABLE_KSERVE_WITH_LLMISVC == "true" ]]; then
        export KSERVE_OVERLAY_DIR=test-modelcache
      fi
      export INSTALL_LLMISVC_CONFIGS=true
    fi
    ${REPO_ROOT}/hack/setup/infra/manage.kserve-kustomize.sh
    echo "Installing KServe Runtimes..."
    kubectl apply --server-side=true -k config/overlays/test/clusterresources
  fi

  echo "Applying test env patches to ClusterServingRuntimes..."
  kubectl patch clusterservingruntime kserve-huggingfaceserver --type=json -p='[
    {"op":"add","path":"/spec/containers/0/env/-","value":{"name":"TOKIO_WORKER_THREADS","value":"1"}},
    {"op":"add","path":"/spec/containers/0/env/-","value":{"name":"HF_HUB_DISABLE_XET","value":"1"}},
    {"op":"add","path":"/spec/containers/0/env/-","value":{"name":"HF_HUB_ENABLE_HF_TRANSFER","value":"0"}}
  ]'

  kubectl get events -A

  echo "Waiting for seaweedfs to be ready ..."
  kubectl rollout status deployment/seaweedfs -n kserve --timeout=120s

  echo "Add testing models to s3 storage ..."
  if [[ -n "${OPT_125M_CACHE_IMAGE:-}" ]]; then
    OPT_125M_CACHE_IMAGE="${OPT_125M_CACHE_IMAGE}" envsubst '${OPT_125M_CACHE_IMAGE}' \
      < config/overlays/test/s3-local-backend/seaweedfs-init-job-from-cache.yaml | kubectl apply -n kserve -f -
  else
    sed "s|kserve/storage-initializer:latest|${KO_DOCKER_REPO:-kserve}/${STORAGE_INIT_IMG:-storage-initializer}:${TAG:-latest}|g" \
      config/overlays/test/s3-local-backend/seaweedfs-init-job.yaml | kubectl apply -n kserve -f -
  fi
  if ! kubectl wait --for=condition=complete --timeout=900s job/s3-init -n kserve; then
    echo "S3 init job failed. Pod status and logs:"
    kubectl get pods -l job-name=s3-init -n kserve
    kubectl describe pods -l job-name=s3-init -n kserve || true
    kubectl logs -l job-name=s3-init -n kserve --all-containers --tail=50 || true
    exit 1
  fi

  echo "Add storageSpec testing secrets ..."
  kubectl apply -f config/overlays/test/s3-local-backend/storage-config-secret.yaml -n "$E2E_NS"

  echo "Configuring S3 credentials for model downloads ..."
  kubectl apply -f config/overlays/test/s3-local-backend/seaweedfs-s3-creds-secret.yaml -n "$E2E_NS"
  kubectl patch serviceaccount default -n "$E2E_NS" \
    --type=merge -p='{"secrets": [{"name": "seaweedfs-s3-creds"}]}'

  if [[ "$E2E_WORKERS" -gt 0 ]]; then
    for i in $(seq 0 $((E2E_WORKERS - 1))); do
      WORKER_NS="${E2E_NS}-gw${i}"
      kubectl apply -f config/overlays/test/s3-local-backend/storage-config-secret.yaml -n "$WORKER_NS"
      kubectl apply -f config/overlays/test/s3-local-backend/seaweedfs-s3-creds-secret.yaml -n "$WORKER_NS"
      kubectl patch serviceaccount default -n "$WORKER_NS" \
        --type=merge -p='{"secrets": [{"name": "seaweedfs-s3-creds"}]}' 2>/dev/null || true
    done
  fi
else
  if [[ $INSTALL_METHOD == "helm" ]]; then
    export SET_KSERVE_VERSION=${TAG}
    export USE_LOCAL_CHARTS=true
    export ENABLE_KSERVE=false
    export LLMISVC_EXTRA_ARGS="--set kserve.llmisvc.controller.imagePullPolicy=IfNotPresent" 
    ${REPO_ROOT}/hack/setup/infra/manage.kserve-helm.sh
  else
    export SET_KSERVE_VERSION=${TAG}
    export INSTALL_RUNTIMES=false
    export INSTALL_LLMISVC_CONFIGS=true
    export KSERVE_OVERLAY_DIR=test-llmisvc
    export ENABLE_LLMISVC=true
    export ENABLE_KSERVE=false
    ${REPO_ROOT}/hack/setup/infra/manage.kserve-kustomize.sh
  fi

  echo "Deploying SeaweedFS for LLMISVC model caching ..."
  kubectl apply -f "${REPO_ROOT}/config/overlays/test/s3-local-backend/mlpipeline-s3-artifact-secret.yaml" -n kserve
  kubectl apply -f "${REPO_ROOT}/config/overlays/test/s3-local-backend/seaweedfs-deployment.yaml" -n kserve
  sed "s/namespace: seaweedfs/namespace: kserve/" \
    "${REPO_ROOT}/config/overlays/test/s3-local-backend/seaweedfs-service.yaml" | kubectl apply -n kserve -f -

  echo "Waiting for seaweedfs to be ready ..."
  kubectl rollout status deployment/seaweedfs -n kserve --timeout=120s

  echo "Pre-caching opt-125m model in SeaweedFS ..."
  if [[ -n "${OPT_125M_CACHE_IMAGE:-}" ]]; then
    OPT_125M_CACHE_IMAGE="${OPT_125M_CACHE_IMAGE}" envsubst '${OPT_125M_CACHE_IMAGE}' \
      < "${REPO_ROOT}/config/overlays/test/s3-local-backend/seaweedfs-init-job-from-cache.yaml" | kubectl apply -n kserve -f -
  else
    sed "s|kserve/storage-initializer:latest|${KO_DOCKER_REPO:-kserve}/${STORAGE_INIT_IMG:-storage-initializer}:${TAG:-latest}|g" \
      "${REPO_ROOT}/config/overlays/test/s3-local-backend/seaweedfs-init-job.yaml" | kubectl apply -n kserve -f -
  fi
  if ! kubectl wait --for=condition=complete --timeout=900s job/s3-init -n kserve; then
    echo "S3 init job failed. Pod status and logs:"
    kubectl get pods -l job-name=s3-init -n kserve
    kubectl describe pods -l job-name=s3-init -n kserve || true
    kubectl logs -l job-name=s3-init -n kserve --all-containers --tail=50 || true
    exit 1
  fi

  echo "Configuring S3 credentials in test namespace ..."
  kubectl apply -f "${REPO_ROOT}/config/overlays/test/s3-local-backend/seaweedfs-s3-creds-secret.yaml" -n "$E2E_NS"
  kubectl patch serviceaccount default -n "$E2E_NS" \
    --type=merge -p='{"secrets": [{"name": "seaweedfs-s3-creds"}]}'

  if [[ "$E2E_WORKERS" -gt 0 ]]; then
    for i in $(seq 0 $((E2E_WORKERS - 1))); do
      WORKER_NS="${E2E_NS}-gw${i}"
      kubectl apply -f "${REPO_ROOT}/config/overlays/test/s3-local-backend/seaweedfs-s3-creds-secret.yaml" -n "$WORKER_NS"
      kubectl patch serviceaccount default -n "$WORKER_NS" \
        --type=merge -p='{"secrets": [{"name": "seaweedfs-s3-creds"}]}' 2>/dev/null || true
    done
  fi
fi

ENABLE_KEDA="${ENABLE_KEDA:-false}"
if [[ $ENABLE_LLMISVC == "true" ]] && [[ $ENABLE_KEDA == "true" ]]; then
  echo "Patching inferenceservice-config with autoscaling-wva-controller-config for KEDA..."
  kubectl patch configmap inferenceservice-config -n kserve --type merge -p '{
    "data": {
      "autoscaling-wva-controller-config": "{\"prometheus\":{\"url\":\"https://prometheus-kube-prometheus-prometheus.monitoring:9090\",\"tlsInsecureSkipVerify\":true}}"
    }
  }'
  echo "Restarting LLMISVC controller to pick up new config..."
  kubectl rollout restart deployment llmisvc-controller-manager -n kserve
  kubectl rollout status deployment llmisvc-controller-manager -n kserve --timeout=120s
  # The old pod from the previous ReplicaSet is terminated asynchronously
  # after rollout completes. Wait for it to be fully removed so it doesn't
  # trip the readiness gate below while still in Running phase.
  echo "Waiting for old controller pod to terminate..."
  for i in $(seq 1 30); do
    pod_count=$(kubectl get pods -l control-plane=llmisvc-controller-manager -n kserve --no-headers 2>/dev/null | wc -l)
    if [ "$pod_count" -le 1 ]; then
      break
    fi
    sleep 2
  done
fi

echo "Show inferenceservice-config configmap..."
kubectl get configmap inferenceservice-config -n kserve

echo "Waiting for all running pods in kserve namespace to be ready..."
kubectl wait --for=condition=Ready pods --field-selector=status.phase=Running --all -n kserve --timeout=180s || {
  echo "ERROR: Pods not ready after 180s. Tests may fail."
  kubectl get pods -n kserve
  exit 1
}
echo "KServe setup complete."
