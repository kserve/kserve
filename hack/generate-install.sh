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
)

TAG=$1

if [[ ! " ${RELEASES[@]} " =~ " ${TAG} " ]]; then
    echo "Expected \$1 to be one of $RELEASES"
    exit 1
fi

INSTALL_DIR=./install/$TAG

HELM_KSERVE_CRD_DIR=charts/kserve-crd
HELM_KSERVE_RESOURCES_DIR=charts/kserve-resources
KUBEFLOW_OVERLAY_DIR=config/overlays/kubeflow

mkdir -p $INSTALL_DIR

generate_yaml() {
    local helm_dir=$1
    local namespace=$2
    local output_path=$3
    shift 3
    helm template "$helm_dir" --namespace "$namespace" "$@" | yq eval -P --indent 2 - > "$output_path"
}

append_yaml() {
    local helm_dir=$1
    local namespace=$2
    local output_path=$3
    shift 3
    helm template "$helm_dir" --namespace "$namespace" "$@" | yq eval -P --indent 2 - | sed '1s/^/---\n/' >> "$output_path"
}

# Generate kserve.yaml
generate_yaml $HELM_KSERVE_CRD_DIR kserve $INSTALL_DIR/kserve.yaml
append_yaml $HELM_KSERVE_RESOURCES_DIR kserve $INSTALL_DIR/kserve.yaml \
    --set kserve.servingruntime.enabled=false \
    --set kserve.storage.enabled=false \
    --set kserve.localmodel.enabled=true

# Generate kserve_kubeflow.yaml
generate_yaml $HELM_KSERVE_CRD_DIR kubeflow $KUBEFLOW_OVERLAY_DIR/kserve.yaml
append_yaml $HELM_KSERVE_RESOURCES_DIR kubeflow $KUBEFLOW_OVERLAY_DIR/kserve.yaml \
    --set kserve.controller.gateway.ingressGateway.gateway="kubeflow/kubeflow-gateway" \
    --set kserve.servingruntime.enabled=false \
    --set kserve.storage.enabled=false \
    --set kserve.localmodel.enabled=true
kubectl kustomize $KUBEFLOW_OVERLAY_DIR | yq eval -P --indent 2 > $INSTALL_DIR/kserve_kubeflow.yaml

# Generate kserve-cluster-resources.yaml
generate_yaml $HELM_KSERVE_RESOURCES_DIR "" $INSTALL_DIR/kserve-cluster-resources.yaml \
    --show-only templates/clusterservingruntimes.yaml \
    --show-only templates/clusterstoragecontainer.yaml
