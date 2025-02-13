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

# The script will install KServe dependencies in the GH Actions environment.
# (Istio, Knative, cert-manager, kustomize, yq)

set -o errexit
set -o nounset
set -o pipefail

sed -i -e "s/*defaultVersion/${GITHUB_SHA}/g" charts/kserve-resources/values.yaml

cat ./charts/kserve-resources/values.yaml

make deploy-helm

echo "Get events of all pods ..."
kubectl get events -A

echo "Creating a namespace kserve-ci-test ..."
kubectl create namespace kserve-ci-e2e-test

echo "Installing KServe Python SDK ..."
python3 -m pip install --upgrade pip
pushd python/kserve >/dev/null
    poetry install --with=test --no-interaction
popd
