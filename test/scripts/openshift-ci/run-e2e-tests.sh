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

# This is a helper script to run E2E tests on the openshift-ci operator.
# This script assumes to be run inside a container/machine that has
# python pre-installed and the `oc` command available. Additional tooling,
# like kustomize and the mc client are installed by the script if not available.
# The oc CLI is assumed to be configured with the credentials of the
# target cluster. The target cluster is assumed to be a clean cluster.
set -o errexit
set -o nounset
set -o pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/common.sh"
PROJECT_ROOT="$(find_project_root "$SCRIPT_DIR")"

readonly MARKERS="${1:-raw}"
readonly PARALLELISM="${2:-1}"

readonly DEPLOYMENT_PROFILE="${3:-raw}"
validate_deployment_profile "${DEPLOYMENT_PROFILE}"

# Map deployment profile to network layer for pytest
case "${DEPLOYMENT_PROFILE}" in
  serverless) NETWORK_LAYER="istio" ;;
  llm-d)      NETWORK_LAYER="gateway-api" ;;
  *)          NETWORK_LAYER="openshift-route" ;;
esac

export GATEWAY_CLASS_NAME=${GATEWAY_CLASS_NAME:-"openshift-default"}
# Detect the correct InferencePool API group from the cluster version.
# setup-kserve.sh does this too, but its export doesn't survive across CI steps.
if [[ -z "${INFERENCE_POOL_GROUP:-}" ]]; then
  source "$SCRIPT_DIR/version.sh"
  server_version=$(get_openshift_server_version)
  ocp_major_minor=$(echo "$server_version" | awk -F. '{print $1"."$2}')
  if awk "BEGIN{exit !($ocp_major_minor <= 4.20)}"; then
    export INFERENCE_POOL_GROUP="inference.networking.x-k8s.io"
  else
    export INFERENCE_POOL_GROUP="inference.networking.k8s.io"
  fi
  echo "INFERENCE_POOL_GROUP=${INFERENCE_POOL_GROUP} (detected from OCP ${server_version})"
fi
export GATEWAY_PROXY_MEMORY="${GATEWAY_PROXY_MEMORY:-2Gi}"
if [[ -n "${PYTHONPATH:-}" ]]; then
  export PYTHONPATH="${PYTHONPATH}:${PROJECT_ROOT}/test/e2e"
else
  export PYTHONPATH="${PROJECT_ROOT}/test/e2e"
fi
export PYTEST_ARGS="${PYTEST_ARGS:-} -p common.gateway_proxy_istio"
export RUN_AS_NON_ROOT="${RUN_AS_NON_ROOT:-true}"
: ${LLMISVC_DEFAULT_ANNOTATIONS:='{"security.opendatahub.io/enable-auth":"false"}'}
export LLMISVC_DEFAULT_ANNOTATIONS
export KUBE_CLI=${KUBE_CLI_COMMAND:-oc}

export GITHUB_SHA=stable # Need to use stable as this is what the CI tags the images to for success-200 and error-404
export BUILD_GRAPH_IMAGES="${BUILD_GRAPH_IMAGES:-true}"
export RUNNING_LOCAL="${RUNNING_LOCAL:-false}"
export SKIP_DELETION_ON_FAILURE="${SKIP_DELETION_ON_FAILURE:=true}"

# Export the controller namespace so that E2E tests
# (e.g. storage version migration) can find the controller.
export KSERVE_NAMESPACE=${KSERVE_NAMESPACE:-"kserve"}
export KEDA_NAMESPACE=${KEDA_NAMESPACE:-"openshift-keda"}
export KEDA_OPERATOR_POD_LABEL=${KEDA_OPERATOR_POD_LABEL:-"app=keda-operator"}

if [[ "$RUNNING_LOCAL" == "true" ]]; then
  export CUSTOM_MODEL_GRPC_IMG_TAG=kserve/custom-model-grpc:latest
  export IMAGE_TRANSFORMER_IMG_TAG=kserve/image-transformer:latest
  export GITHUB_SHA=master
fi

cp ./test/e2e/conftest.py ./test/e2e/conftest.py.bak
trap 'mv ./test/e2e/conftest.py.bak ./test/e2e/conftest.py 2>/dev/null || true' EXIT

: "${SETUP_E2E:=true}"
if [ "$SETUP_E2E" = "true" ]; then
  echo "Installing on cluster"
  pushd $PROJECT_ROOT >/dev/null
  ./test/scripts/openshift-ci/setup-e2e-tests.sh "${MARKERS}" 2>&1 | tee ./test/scripts/openshift-ci/setup-e2e-tests-"${MARKERS// /_}".log
  popd
fi

export OPT_125M_MODEL_URI="${OPT_125M_MODEL_URI:-s3://example-models/facebook/opt-125m}"

# Use certify go module to get the CA certs
# For serverless it is configured here: infra/deploy.serverless.sh
# For Raw: setup-e2e-tests.sh
if [ ! -s "/tmp/ca.crt" ]; then
  echo "Failed to extract CA certificate, using system defaults... tests might fail as certificates are not present."
  unset REQUESTS_CA_BUNDLE
else
  echo "CA certificate extracted"
  export REQUESTS_CA_BUNDLE="/tmp/ca.crt"
  echo "REQUESTS_CA_BUNDLE=${REQUESTS_CA_BUNDLE}"
fi

echo "Run E2E tests: ${MARKERS}"
pushd $PROJECT_ROOT >/dev/null
./test/scripts/gh-actions/run-e2e-tests.sh "${MARKERS}" "${PARALLELISM}" "${NETWORK_LAYER}" 2>&1 | tee ./test/scripts/openshift-ci/run-e2e-tests-"${MARKERS// /_}".log
popd

