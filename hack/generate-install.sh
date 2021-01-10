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

RELEASES=(
    "0.1.0"
    "0.2.0"
    "0.2.1"
    "0.2.2"
    "v0.3.0"
    "v0.4.0"
    "v0.4.1"
    "v0.5.0-rc0"
    "v0.5.0-rc1"
    "v0.5.0-rc2"
)

TAG=$1

if [[ ! " ${RELEASES[@]} " =~ " ${TAG} " ]]; then
    echo "Expected \$1 to be one of $RELEASES"
    exit 1
fi

INSTALL_DIR=./install/$TAG
INSTALL_PATH=$INSTALL_DIR/kfserving.yaml

mkdir -p $INSTALL_DIR
kustomize build config/default | sed s/:latest/:$TAG/ > $INSTALL_PATH
