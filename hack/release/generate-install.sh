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

set -x

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd 2>/dev/null)"
source "${SCRIPT_DIR}/../setup/common.sh"
REPO_ROOT="$(find_repo_root "${SCRIPT_DIR}")"
source "${REPO_ROOT}/kserve-deps.env"
source "${REPO_ROOT}/hack/setup/global-vars.env"

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
    "v0.5.0"
    "v0.5.1"
    "v0.6.0-rc0"
    "v0.6.0"
    "v0.6.1"
    "v0.7.0-rc0"
    "v0.7.0"
    "v0.8.0-rc0"
    "v0.8.0"
    "v0.9.0-rc0"
    "v0.9.0"
    "v0.10.0-rc0"
    "v0.10.0-rc1"
    "v0.10.0"
    "v0.10.1"
    "v0.11.0-rc0"
    "v0.11.0-rc1"
    "v0.11.0"
    "v0.11.1"
    "v0.12.0-rc0"
    "v0.12.0-rc1"
    "v0.12.0"
    "v0.12.1"
    "v0.13.0-rc0"
    "v0.13.0"
    "v0.14.0-rc0"
    "v0.14.0-rc1"
    "v0.14.0"
    "v0.15.0-rc0"
    "v0.15.0-rc1"
    "v0.15.0"
    "v0.15.1"
    "v0.15.2"
    "v0.16.0-rc0"
    "v0.16.0-rc1"
    "v0.16.0"
    "v0.17.0-rc0"
    "v0.17.0-rc1"
)

TAG=$1

if [[ ! " ${RELEASES[@]} " =~ " ${TAG} " ]]; then
    echo "Expected \$1 to be one of $RELEASES"
    exit 1
fi

INSTALL_DIR=${REPO_ROOT}/install/$TAG
INSTALL_CRD_PATH=$INSTALL_DIR/kserve-crds.yaml
INSTALL_PATH=$INSTALL_DIR/kserve.yaml
KUBEFLOW_INSTALL_PATH=$INSTALL_DIR/kserve_kubeflow.yaml
RUNTIMES_INSTALL_PATH=$INSTALL_DIR/kserve-cluster-resources.yaml

mkdir -p $INSTALL_DIR

# Copy all quick-install scripts
cp -R ${REPO_ROOT}/hack/setup/quick-install/*-install.sh $INSTALL_DIR/.
cp -R ${REPO_ROOT}/hack/setup/quick-install/*-manifests.sh $INSTALL_DIR/.

# Update image tags in *-with-manifests.sh files (embedded manifests need release version)
for script in $INSTALL_DIR/*-with-manifests.sh; do
  if [ -f "$script" ]; then
    sed -i "s/:latest/:$TAG/g" "$script"
  fi
done

# Generate YAML manifests with release version
# Dependency install script creates a namespace
kustomize build ${REPO_ROOT}/config/crd/full > $INSTALL_CRD_PATH
kustomize build ${REPO_ROOT}/config/overlays/all | yq 'select(.kind != "Namespace")' | sed s/:latest/:$TAG/g > $INSTALL_PATH
kustomize build ${REPO_ROOT}/config/overlays/kubeflow | sed s/:latest/:$TAG/g > $KUBEFLOW_INSTALL_PATH
kustomize build ${REPO_ROOT}/config/clusterresources | sed s/:latest/:$TAG/g > $RUNTIMES_INSTALL_PATH

# Update ingressGateway in inferenceservice configmap as 'kubeflow/kubeflow-gateway'
yq -i 'select(.metadata.name == "inferenceservice-config").data.ingress |= (fromjson | .ingressGateway = "kubeflow/kubeflow-gateway" | tojson)' $KUBEFLOW_INSTALL_PATH
