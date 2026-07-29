#!/usr/bin/env bash
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#
# Installs KServe directly via kustomize (manual install path, no operator).
# Called by setup-kserve.sh when OPERATOR_TYPE is empty.
#
# Env-var interface:
#   KSERVE_NAMESPACE          target namespace (default: kserve)
#   KSERVE_CONTROLLER_IMAGE   override; default from config/overlays/odh/params.env
#   KSERVE_AGENT_IMAGE        override; default from params.env
#   KSERVE_ROUTER_IMAGE       override; default from params.env
#   STORAGE_INITIALIZER_IMAGE override; default from params.env
#   LLMISVC_CONTROLLER_IMAGE  override; default from params.env
#
# Positional args:
#   $1   E2E marker -- gates llm-d controller restart (pass empty string to skip)
#
# Note: SeaweedFS is bundled in config/overlays/odh-test and is NOT deployed separately.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
source "${SCRIPT_DIR}/common.sh"

: "${KSERVE_NAMESPACE:=kserve}"

# Install kustomize/yq if not already available (idempotent; also done by callers)
"${PROJECT_ROOT}/hack/setup/cli/install-kustomize.sh"
make -C "${PROJECT_ROOT}" yq
export PATH="${PROJECT_ROOT}/bin:${PATH}"

PARAMS_ENV="${PROJECT_ROOT}/config/overlays/odh/params.env"

# When QUAY_REPO is set, prefer locally-built images (tagged :${GITHUB_SHA:-master})
# over the params.env defaults. This covers both BUILD_KSERVE_IMAGES=true (where
# build-kserve-images.sh exports these vars) and BUILD_KSERVE_IMAGES=false (where
# the vars are not yet set but the images have already been pushed).
if [[ -n "${QUAY_REPO:-}" ]]; then
  _repo="${QUAY_REPO}"
  _tag="${GITHUB_SHA:-master}"
  : "${KSERVE_CONTROLLER_IMAGE:=${_repo}/kserve-controller:${_tag}}"
  : "${KSERVE_AGENT_IMAGE:=${_repo}/kserve-agent:${_tag}}"
  : "${KSERVE_ROUTER_IMAGE:=${_repo}/kserve-router:${_tag}}"
  : "${STORAGE_INITIALIZER_IMAGE:=${_repo}/kserve-storage-initializer:${_tag}}"
  : "${LLMISVC_CONTROLLER_IMAGE:=${_repo}/llmisvc-controller:${_tag}}"
else
  : "${KSERVE_CONTROLLER_IMAGE:=$(grep '^kserve-controller=' "${PARAMS_ENV}" | cut -d= -f2-)}"
  : "${KSERVE_AGENT_IMAGE:=$(grep '^kserve-agent=' "${PARAMS_ENV}" | cut -d= -f2-)}"
  : "${KSERVE_ROUTER_IMAGE:=$(grep '^kserve-router=' "${PARAMS_ENV}" | cut -d= -f2-)}"
  : "${STORAGE_INITIALIZER_IMAGE:=$(grep '^kserve-storage-initializer=' "${PARAMS_ENV}" | cut -d= -f2-)}"
  : "${LLMISVC_CONTROLLER_IMAGE:=$(grep '^llmisvc-controller=' "${PARAMS_ENV}" | cut -d= -f2-)}"
fi

echo "KSERVE_CONTROLLER_IMAGE=$KSERVE_CONTROLLER_IMAGE"
echo "LLMISVC_CONTROLLER_IMAGE=$LLMISVC_CONTROLLER_IMAGE"
echo "KSERVE_AGENT_IMAGE=$KSERVE_AGENT_IMAGE"
echo "KSERVE_ROUTER_IMAGE=$KSERVE_ROUTER_IMAGE"
echo "STORAGE_INITIALIZER_IMAGE=$STORAGE_INITIALIZER_IMAGE"

echo "Installing KServe via kustomize..."

cp "${PARAMS_ENV}" "${PARAMS_ENV}.bak"
trap "mv '${PARAMS_ENV}.bak' '${PARAMS_ENV}' 2>/dev/null || true" EXIT

sed -i "s|kserve-controller=.*|kserve-controller=${KSERVE_CONTROLLER_IMAGE}|" "${PARAMS_ENV}"
sed -i "s|llmisvc-controller=.*|llmisvc-controller=${LLMISVC_CONTROLLER_IMAGE}|" "${PARAMS_ENV}"
sed -i "s|kserve-agent=.*|kserve-agent=${KSERVE_AGENT_IMAGE}|" "${PARAMS_ENV}"
sed -i "s|kserve-router=.*|kserve-router=${KSERVE_ROUTER_IMAGE}|" "${PARAMS_ENV}"
sed -i "s|kserve-storage-initializer=.*|kserve-storage-initializer=${STORAGE_INITIALIZER_IMAGE}|" "${PARAMS_ENV}"

echo "=== Final params.env"
cat "${PARAMS_ENV}"

kustomize build "${PROJECT_ROOT}/config/overlays/odh-crds" | oc apply --server-side=true --force-conflicts -f -

ODH_MANIFESTS=$(kustomize build "${PROJECT_ROOT}/config/overlays/odh-test")

# Apply CRDs first and wait for them to be established before applying the rest
echo "${ODH_MANIFESTS}" | awk '/^apiVersion: apiextensions\.k8s\.io/{found=1} found{print} /^---/{if(found) found=0}' |
  oc apply --server-side=true --force-conflicts -f -

echo "Waiting for CRDs to be established..."
wait_for_crd inferenceservices.serving.kserve.io 90s
wait_for_crd llminferenceserviceconfigs.serving.kserve.io 90s
wait_for_crd clusterstoragecontainers.serving.kserve.io 90s
wait_for_crd datascienceclusters.datasciencecluster.opendatahub.io 90s

# Apply all resources (LLMInferenceServiceConfig may fail webhook validation initially, will retry after)
echo "Applying all resources..."
echo "${ODH_MANIFESTS}" | oc apply --server-side=true --force-conflicts -f - || true

echo "Waiting for llmisvc-controller-manager to be ready..."
wait_for_pod_ready "${KSERVE_NAMESPACE}" "control-plane=llmisvc-controller-manager" 600s

# We cannot apply individual overlays with `kustomize build "${PROJECT_ROOT}/config/llmisvcconfig"` because it will
# override ODH images that are only injected for the odh overlay.
echo "Re-Applying all resources..."
echo "${ODH_MANIFESTS}" | oc apply --server-side=true --force-conflicts -f -

echo "Applying DSC/DSCI resources..."
oc apply -f "${PROJECT_ROOT}/config/overlays/odh-test/dsci.yaml"
oc apply -f "${PROJECT_ROOT}/config/overlays/odh-test/dsc.yaml"

echo "KServe manual installation complete"
