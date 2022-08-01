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

# The script is used to build all the KServe images.

# TODO: Implement selective building and tag replacement based on modified code.

set -o errexit
set -o nounset
set -o pipefail
echo "Github SHA ${GITHUB_SHA}"
export CONTROLLER_IMG=kserve/kserve-controller:${GITHUB_SHA}
STORAGE_INIT_IMG=kserve/storage-initializer:${GITHUB_SHA}
AGENT_IMG=kserve/agent:${GITHUB_SHA}


echo "Building Kserve controller image"
docker build . -t ${CONTROLLER_IMG}

echo "Building agent image"
docker build -f agent.Dockerfile . -t ${AGENT_IMG}

pushd python >/dev/null
  echo "Building storage initializer"
  docker build -t ${STORAGE_INIT_IMG} -f storage-initializer.Dockerfile .
popd

echo "Done building images"
