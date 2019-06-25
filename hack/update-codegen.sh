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

STARTUP_DIR="$( cd "$( dirname "$0" )" && pwd )"

if [ -z "${GOPATH:-}" ]; then
    export GOPATH=$(go env GOPATH)
fi

CODEGEN_PKG=${STARTUP_DIR}/../vendor/k8s.io/code-generator

echo ${CODEGEN_PKG}

${CODEGEN_PKG}/generate-groups.sh all "github.com/kubeflow/kfserving/pkg/client" "github.com/kubeflow/kfserving/pkg/apis" serving:v1alpha1