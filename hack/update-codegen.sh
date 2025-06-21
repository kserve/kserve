#!/bin/bash

# Copyright 2019 The Kubeflow Authors.
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

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_DIR="$(dirname "${BASH_SOURCE[0]}")"
SCRIPT_ROOT="${SCRIPT_DIR}/.."
CODEGEN_VERSION=$(cd "${SCRIPT_ROOT}" && grep 'k8s.io/code-generator' go.mod | awk '{print $2}')
KUBE_CODEGEN_TAG=${CODEGEN_VERSION}

# For debugging purposes
echo "Codegen version ${CODEGEN_VERSION}"

if [ -z "${GOPATH:-}" ]; then
    GOPATH=$(go env GOPATH)
    export GOPATH
fi
CODEGEN_PKG="$GOPATH/pkg/mod/k8s.io/code-generator@${CODEGEN_VERSION}"
THIS_PKG="github.com/kserve/kserve"

# shellcheck source=/dev/null
source "${CODEGEN_PKG}/kube_codegen.sh"

kube::codegen::gen_helpers \
    --boilerplate "${SCRIPT_ROOT}/hack/boilerplate.go.txt" \
    "${SCRIPT_ROOT}"

kube::codegen::gen_client \
    --with-watch \
    --output-dir "${SCRIPT_ROOT}/pkg/client" \
    --output-pkg "${THIS_PKG}/pkg/client" \
    --boilerplate "${SCRIPT_ROOT}/hack/boilerplate.go.txt" \
    "${SCRIPT_ROOT}/pkg/apis"
