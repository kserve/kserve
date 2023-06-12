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

SCRIPT_DIR="$( cd -- "$( dirname -- "${BASH_SOURCE[0]:-$0}"; )" &> /dev/null && pwd 2> /dev/null; )";

KNATIVE_KOURIER_VERSION="knative-v1.10.0"
KNATIVE_VERSION="knative-v1.10.1"
CERT_MANAGER_VERSION="v1.5.0"
YQ_VERSION="v4.28.1"

echo "Installing yq ..."
wget https://github.com/mikefarah/yq/releases/download/${YQ_VERSION}/yq_linux_amd64 -O /usr/local/bin/yq && chmod +x /usr/local/bin/yq

echo "Installing Knative serving and Kourier..."
pushd "${SCRIPT_DIR}"/../../overlays/knative/default >/dev/null
  curl -s -O -L https://github.com/knative/serving/releases/download/${KNATIVE_VERSION}/serving-core.yaml
  curl -s -O -L https://github.com/knative/net-kourier/releases/download/${KNATIVE_KOURIER_VERSION}/release.yaml
  # Kustomize does not work with integer map keys
  sed -i 's/8443:/"8443":/g' release.yaml
popd

for i in 1 2 3 ; do kubectl apply -k test/overlays/knative/overlays/kourier && break || sleep 15; done

echo "Waiting for Knative to be ready ..."
kubectl wait --for=condition=Ready pods --all --timeout=300s -n knative-serving -l 'app in (webhook, activator,autoscaler,autoscaler-hpa,controller,net-istio-controller,net-istio-webhook)'

# sleep to avoid knative webhook timeout error
sleep 5
# Retry if configmap patch fails
for i in 1 2 3; do
  # Skip tag resolution for certain domains
  kubectl patch cm config-deployment --patch '{"data":{"registries-skipping-tag-resolving":"nvcr.io,index.docker.io"}}' -n knative-serving && break || sleep 15
done

echo "Patching knative external domain ..."
# Patch the external domain as the default domain svc.cluster.local is not exposed on ingress (from knative 1.8)
for i in 1 2 3; do kubectl patch cm config-domain --patch '{"data":{"example.com":""}}' -n knative-serving && break || sleep 15; done
kubectl describe cm config-domain -n knative-serving

echo "Configuring Knative Serving to use kourier ..."
for i in 1 2 3; do
kubectl patch configmap/config-network \
  -n knative-serving \
  --type merge \
  -p '{"data":{"ingress.class":"kourier.ingress.networking.knative.dev"}}'
done

echo "Installing cert-manager ..."
kubectl create namespace cert-manager
sleep 2
kubectl apply --validate=false -f https://github.com/jetstack/cert-manager/releases/download/${CERT_MANAGER_VERSION}/cert-manager.yaml

echo "Waiting for cert-manager to be ready ..."
kubectl wait --for=condition=ready pod -l 'app in (cert-manager,webhook)' --timeout=180s -n cert-manager
