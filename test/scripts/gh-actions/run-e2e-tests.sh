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

# The script is used to deploy knative and kserve, and run e2e tests.
# Usage: run-e2e-tests.sh $MARKER $PARALLELISM $NETWORK_LAYER

set -o errexit
set -o nounset
set -o pipefail

echo "Starting E2E functional tests ..."
if [ $# -eq 2 ]; then
  echo "Parallelism requested for pytest is $2"
else
  echo "No parallelism requested for pytest. Will use default value of 1"
fi

MARKER="${1}"
PARALLELISM="${2:-1}"
NETWORK_LAYER="${3:-'istio'}"

source python/kserve/.venv/bin/activate

pushd test/e2e >/dev/null
  if [[ $MARKER == "raw" && $NETWORK_LAYER == "istio-ingress" ]]; then
    echo "Skipping explainer tests for raw deployment with ingress"
    pytest -m "$MARKER" --ignore=qpext --log-cli-level=INFO -n $PARALLELISM --dist worksteal --network-layer $NETWORK_LAYER --ignore=explainer/
  else
    pytest -m "$MARKER" --ignore=qpext --log-cli-level=INFO -n $PARALLELISM --dist worksteal --network-layer $NETWORK_LAYER
  fi
popd
