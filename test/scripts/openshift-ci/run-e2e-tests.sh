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

MY_PATH=$(dirname "$0")
PROJECT_ROOT=$MY_PATH/../../../

SETUP_E2E=true
if $SETUP_E2E; then
  echo "Installing on cluster"
  pushd $PROJECT_ROOT >/dev/null
  ./test/scripts/openshift-ci/setup-e2e-tests.sh "$1"
  popd
fi

echo "Run E2E tests: $1"
pushd $PROJECT_ROOT >/dev/null
# Note: The following images are set by openshift-ci. Uncomment if you are running on your own machine.
#export CUSTOM_MODEL_GRPC_IMG_TAG=kserve/custom-model-grpc:latest
#export IMAGE_TRANSFORMER_IMG_TAG=kserve/image-transformer:latest

export GITHUB_SHA=$(git rev-parse HEAD)
export CI_USE_ISVC_HOST="1"
./test/scripts/gh-actions/run-e2e-tests.sh "$1"
popd
