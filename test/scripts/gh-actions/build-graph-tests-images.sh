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

# The script is used to build all the queue-proxy extension image.

set -o errexit
set -o nounset
set -o pipefail
echo "Github SHA ${GITHUB_SHA}"
export SUCCESS_200_ISVC_IMG=kserve/success-200-isvc:${GITHUB_SHA}
export ERROR_404_ISVC_IMG=kserve/error-404-isvc:${GITHUB_SHA}

pushd python >/dev/null
echo "Building success_200_isvc image"
docker buildx build -t ${SUCCESS_200_ISVC_IMG} -f success_200_isvc.Dockerfile .
echo "Done building success_200_isvc image"
echo "Building error_404_isvc image"
docker buildx build -t ${ERROR_404_ISVC_IMG} -f error_404_isvc.Dockerfile .
echo "Done building error_404_isvc image"
popd
echo "Done building images"

