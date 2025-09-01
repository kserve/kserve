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

# The script is used to deploy knative and kserve, and run e2e tests.


# Update KServe configurations to use the correct tag. This replaces all 'latest' entries in the configmap include the
# agent and storage-initializer.
sed -i -e "s/latest/${GITHUB_SHA}/g" config/overlays/test/configmap/inferenceservice.yaml

# Update cluster resources
sed -i -e "s/latest/${GITHUB_SHA}/g" config/overlays/test/clusterresources/kustomization.yaml

# Update controller image tag
sed -i -e "s/latest/${GITHUB_SHA}/g" config/overlays/test/manager_image_patch.yaml

# Update localmodel controller image tag
sed -i -e "s/latest/${GITHUB_SHA}/g" config/overlays/test/localmodel_manager_image_patch.yaml

# Update localmodel agent image tag
sed -i -e "s/latest/${GITHUB_SHA}/g" config/overlays/test/localmodelnode_agent_image_patch.yaml