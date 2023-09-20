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

set -eu

: "${SKLEARN_IMAGE:=kserve/sklearnserver:latest}"
: "${KSERVE_CONTROLLER_IMAGE:=quay.io/opendatahub/kserve-controller:latest}"
: "${KSERVE_AGENT_IMAGE:=quay.io/opendatahub/kserve-agent:latest}"
: "${KSERVE_ROUTER_IMAGE:=quay.io/opendatahub/kserve-router:latest}"
: "${STORAGE_INITIALIZER_IMAGE:=quay.io/opendatahub/kserve-storage-initializer:latest}"

echo "SKLEARN_IMAGE=$SKLEARN_IMAGE"
echo "KSERVE_CONTROLLER_IMAGE=$KSERVE_CONTROLLER_IMAGE"
echo "KSERVE_AGENT_IMAGE=$KSERVE_AGENT_IMAGE"
echo "KSERVE_ROUTER_IMAGE=$KSERVE_ROUTER_IMAGE"
echo "STORAGE_INITIALIZER_IMAGE=$STORAGE_INITIALIZER_IMAGE"

# Create directory for installing tooling
# It is assumed that $HOME/.local/bin is in the $PATH
mkdir -p $HOME/.local/bin
MY_PATH=$(dirname "$0")
PROJECT_ROOT=$MY_PATH/../../../

# If Kustomize is not installed, install it
if ! command -v kustomize &> /dev/null; then
  echo "Installing Kustomize"
  curl -s "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh"  | bash -s -- 5.0.1 $HOME/.local/bin
fi

# If minio CLI is not installed, install it
if ! command -v mc &> /dev/null; then
  echo "Installing Minio CLI"
  curl https://dl.min.io/client/mc/release/linux-amd64/mc --create-dirs -o $HOME/.local/bin/mc
  chmod +x $HOME/.local/bin/mc
fi

#
echo "Installing KServe Python SDK ..."
pushd $PROJECT_ROOT >/dev/null
  ./test/scripts/gh-actions/setup-poetry.sh
  ./test/scripts/gh-actions/check-poetry-lockfile.sh
popd
pushd $PROJECT_ROOT/python/kserve >/dev/null
    poetry install --with=test --no-interaction
popd

# Install KServe stack
echo "Installing OSSM"
$MY_PATH/deploy.ossm.sh
echo "Installing Serverless"
$MY_PATH/deploy.serverless.sh

echo "Installing KServe with Minio"
kustomize build $PROJECT_ROOT/config/overlays/test | \
  sed "s|kserve/storage-initializer:latest|${STORAGE_INITIALIZER_IMAGE}|" | \
  sed "s|kserve/agent:latest|${KSERVE_AGENT_IMAGE}|" | \
  sed "s|kserve/router:latest|${KSERVE_ROUTER_IMAGE}|" | \
  sed "s|kserve/kserve-controller:latest|${KSERVE_CONTROLLER_IMAGE}|" | \
  oc apply -f -
oc wait --for=condition=ready pod -l control-plane=kserve-controller-manager -n kserve --timeout=300s

echo "Add testing models to minio storage ..." # Reference: config/overlays/test/minio/minio-init-job.yaml
curl -L https://storage.googleapis.com/kfserving-examples/models/sklearn/1.0/model/model.joblib -o /tmp/sklearn-model.joblib
oc expose service minio-service -n kserve && sleep 5
MINIO_ROUTE=$(oc get routes -n kserve minio-service -o jsonpath="{.spec.host}")
mc alias set storage http://$MINIO_ROUTE minio minio123
mc mb storage/example-models
mc cp /tmp/sklearn-model.joblib storage/example-models/sklearn/model.joblib
oc delete route -n kserve minio-service

#
echo "Prepare CI namespace and install ServingRuntimes"
cat <<EOF | oc apply -f -
apiVersion: v1
kind: Namespace
metadata:
  name: kserve-ci-e2e-test
  labels:
    testing.kserve.io/add-to-mesh: "true"
EOF

oc apply -f $PROJECT_ROOT/config/overlays/test/minio/minio-user-secret.yaml -n kserve-ci-e2e-test

kustomize build $PROJECT_ROOT/config/overlays/test/runtimes | \
  sed 's/ClusterServingRuntime/ServingRuntime/' | \
  sed "s|kserve/sklearnserver:latest|${SKLEARN_IMAGE}|" | \
  oc apply -n kserve-ci-e2e-test -f -

#
echo "Run E2E tests: $1"
pushd $PROJECT_ROOT >/dev/null
  export GITHUB_SHA=$(git rev-parse HEAD)
  ./test/scripts/gh-actions/run-e2e-tests.sh "$1"
popd
