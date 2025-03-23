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
INSTALL_PATH=$INSTALL_DIR/kserve.yaml
KUBEFLOW_INSTALL_PATH=$INSTALL_DIR/kserve_kubeflow.yaml
RUNTIMES_INSTALL_PATH=$INSTALL_DIR/kserve-cluster-resources.yaml

HELM_KSERVE_CRD_DIR=charts/kserve-crd
HELM_KSERVE_RESOURCES_DIR=charts/kserve-resources
KUBEFLOW_OVERLAY_DIR=config/overlays/kubeflow
KSERVE_OVERLAY_PATH=$KUBEFLOW_OVERLAY_DIR/kserve.yaml

mkdir -p $INSTALL_DIR
# Generate kserve.yaml
helm template $HELM_KSERVE_CRD_DIR \
  --namespace kserve \
  > $INSTALL_PATH
helm template $HELM_KSERVE_RESOURCES_DIR \
  --namespace kserve \
  --set kserve.servingruntime.enabled=false \
  --set kserve.storage.enabled=false \
  >> $INSTALL_PATH
# Generate kserve_kubeflow.yaml
helm template $HELM_KSERVE_CRD_DIR \
  --namespace kubeflow \
  > $KSERVE_OVERLAY_PATH
helm template $HELM_KSERVE_RESOURCES_DIR \
  --namespace kubeflow \
  --set kserve.servingruntime.enabled=false \
  --set kserve.storage.enabled=false \
  >> $KSERVE_OVERLAY_PATH
kubectl kustomize $KUBEFLOW_OVERLAY_DIR | sed s/:latest/:$TAG/ > $KUBEFLOW_INSTALL_PATH
# Generate kserve-cluster-resources.yaml
helm template $HELM_KSERVE_RESOURCES_DIR \
  --show-only templates/clusterservingruntimes.yaml \
  --show-only templates/clusterstoragecontainer.yaml \
  > $RUNTIMES_INSTALL_PATH

# Update ingressGateway in inferenceservice configmap as 'kubeflow/kubeflow-gateway'
yq -i 'select(.metadata.name == "inferenceservice-config").data.ingress |= (fromjson | .ingressGateway = "kubeflow/kubeflow-gateway" | tojson)' $KUBEFLOW_INSTALL_PATH
