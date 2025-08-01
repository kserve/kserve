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
    "v0.15.1"
    "v0.15.2"
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

mkdir -p $INSTALL_DIR
kubectl kustomize config/default | sed s/:latest/:$TAG/ > $INSTALL_PATH
kubectl kustomize config/overlays/kubeflow | sed s/:latest/:$TAG/ > $KUBEFLOW_INSTALL_PATH
kubectl kustomize config/clusterresources | sed s/:latest/:$TAG/ > $RUNTIMES_INSTALL_PATH

# Update ingressGateway in inferenceservice configmap as 'kubeflow/kubeflow-gateway'
yq -i 'select(.metadata.name == "inferenceservice-config").data.ingress |= (fromjson | .ingressGateway = "kubeflow/kubeflow-gateway" | tojson)' $KUBEFLOW_INSTALL_PATH

# Create a temp directory for final CRD
temp_dir=$(mktemp -d)
delimeter_lines=$(cat -n ${INSTALL_PATH} |grep '\-\-\-'|cut -f1)
start_line=1
for end_line in $delimeter_lines
do
  sed -n "${start_line},$((end_line-1))p" "${INSTALL_PATH}" > "${temp_dir}/temp_output_file.yaml"
  start_line=$(( end_line+1 ))
  kind=$(yq '.kind' "${temp_dir}/temp_output_file.yaml")
  plural_name=$(yq  '.spec.names.plural' "${temp_dir}/temp_output_file.yaml")
  if [[ $kind == 'CustomResourceDefinition' ]]
  then
     group=$(yq '.spec.group' "${temp_dir}/temp_output_file.yaml")     
     mv "${temp_dir}/temp_output_file.yaml" "${temp_dir}/${group}_${plural_name}.yaml"
  fi
done

# Copy CRD files to charts crds directory
cp ${temp_dir}/serving.kserve.io_clusterservingruntimes.yaml charts/kserve-crd/templates/serving.kserve.io_clusterservingruntimes.yaml
cp ${temp_dir}/serving.kserve.io_inferenceservices.yaml charts/kserve-crd/templates/serving.kserve.io_inferenceservices.yaml
cp ${temp_dir}/serving.kserve.io_trainedmodels.yaml charts/kserve-crd/templates/serving.kserve.io_trainedmodels.yaml
cp ${temp_dir}/serving.kserve.io_inferencegraphs.yaml charts/kserve-crd/templates/serving.kserve.io_inferencegraphs.yaml
cp ${temp_dir}/serving.kserve.io_servingruntimes.yaml charts/kserve-crd/templates/serving.kserve.io_servingruntimes.yaml
cp ${temp_dir}/serving.kserve.io_clusterstoragecontainers.yaml charts/kserve-crd/templates/serving.kserve.io_clusterstoragecontainers.yaml
cp ${temp_dir}/serving.kserve.io_localmodelnodegroups.yaml charts/kserve-crd/templates/serving.kserve.io_localmodelnodegroups.yaml
cp ${temp_dir}/serving.kserve.io_localmodelcaches.yaml charts/kserve-crd/templates/serving.kserve.io_localmodelcaches.yaml

# Clean temp directory
rm -rf ${temp_dir}
