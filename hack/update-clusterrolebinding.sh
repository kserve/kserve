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

# Usage: update-clusterrolebinding.sh [CONFIG_DIR]

CONFIG_DIR=$1

set -o errexit
set -o nounset
set -o pipefail

TMPFILE="/tmp/tmp_$$"

if [ ! -d "${TMPFILE}" ] 
then
    mkdir ${TMPFILE}
fi

kustomize build ${CONFIG_DIR} -o ${TMPFILE}
for i in ${TMPFILE}/rbac.authorization.k8s.io*clusterrolebinding*.yaml; do kubectl auth reconcile -f $i; done
rm -rf ${TMPFILE}
