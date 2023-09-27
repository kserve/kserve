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

KUBE_ROOT=$(dirname "${BASH_SOURCE[0]}")/..
CODEGEN_VERSION=$(cd "${KUBE_ROOT}" && grep 'k8s.io/code-generator' go.mod | awk '{print $2}')

if [ -z "${GOPATH:-}" ]; then
    export GOPATH=$(go env GOPATH)
fi
CODEGEN_PKG="$GOPATH/pkg/mod/k8s.io/code-generator@${CODEGEN_VERSION}"

chmod +x "${CODEGEN_PKG}/generate-groups.sh"

# We can not generate client-go for v1alpha1 and v1beta1 and add them to the same directory.
# So, we add each to a separate directory.
# Generating files for v1alpha1
"${CODEGEN_PKG}/generate-groups.sh" \
    "deepcopy,client,informer,lister" \
    "github.com/kserve/kserve/pkg/clientv1alpha1" \
    "github.com/kserve/kserve/pkg/apis" \
    "serving:v1alpha1" \
    --go-header-file "${KUBE_ROOT}/hack/boilerplate.go.txt"

# Generating files for v1beta1
"${CODEGEN_PKG}/generate-groups.sh" \
    "deepcopy,client,informer,lister" \
    "github.com/kserve/kserve/pkg/client" \
    "github.com/kserve/kserve/pkg/apis" \
    "serving:v1beta1" \
    --go-header-file "${KUBE_ROOT}/hack/boilerplate.go.txt"
