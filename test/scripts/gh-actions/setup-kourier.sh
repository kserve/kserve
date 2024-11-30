#!/bin/bash

# Copyright 2023 The KServe Authors.
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
# (Kourier, Knative, cert-manager, kustomize, yq)

set -o errexit
set -o nounset
set -o pipefail

CERT_MANAGER_VERSION="v1.15.1"
YQ_VERSION="v4.28.1"
GATEWAY_API_VERSION="v1.2.1"

echo "Installing yq ..."
wget https://github.com/mikefarah/yq/releases/download/${YQ_VERSION}/yq_linux_amd64 -O /usr/local/bin/yq && chmod +x /usr/local/bin/yq

echo "Installing Gateway CRDs ..."
kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/${GATEWAY_API_VERSION}/standard-install.yaml

source  ./test/scripts/gh-actions/install-knative-operator.sh

echo "Installing Knative serving and Kourier..."
kubectl apply -f ./test/overlays/knative/knative-serving-kourier.yaml

echo "Waiting for Knative and Kourier to be ready ..."
kubectl wait --for=condition=Ready -n knative-serving KnativeServing knative-serving --timeout=300s

echo "Installing cert-manager ..."
kubectl create namespace cert-manager
sleep 2
kubectl apply --validate=false -f https://github.com/jetstack/cert-manager/releases/download/${CERT_MANAGER_VERSION}/cert-manager.yaml

echo "Waiting for cert-manager to be ready ..."
kubectl wait --for=condition=ready pod -l 'app in (cert-manager,webhook)' --timeout=180s -n cert-manager
