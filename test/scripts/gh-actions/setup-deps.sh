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

SCRIPT_DIR="$( cd -- "$( dirname -- "${BASH_SOURCE[0]:-$0}"; )" &> /dev/null && pwd 2> /dev/null; )";

ISTIO_VERSION="1.12.0"
KNATIVE_VERSION="knative-v1.4.0"
CERT_MANAGER_VERSION="v1.5.0"
KUSTOMIZE_VERSION="4.2.0"
YQ_VERSION="v4.28.1"

echo "Installing/Updating kustomize ..."
KUSTOMIZE_PATH=$(which kustomize)
rm -rf ${KUSTOMIZE_PATH}
curl -s "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh" | bash -s -- ${KUSTOMIZE_VERSION} ${KUSTOMIZE_PATH::-10}

echo "Installing yq ..."
wget https://github.com/mikefarah/yq/releases/download/${YQ_VERSION}/yq_linux_amd64 -O /usr/local/bin/yq && chmod +x /usr/local/bin/yq

echo "Installing Istio ..."
mkdir istio_tmp
pushd istio_tmp >/dev/null
  curl -L https://istio.io/downloadIstio | ISTIO_VERSION=${ISTIO_VERSION} sh -
  cd istio-${ISTIO_VERSION}
  export PATH=$PWD/bin:$PATH
  istioctl manifest generate > ${SCRIPT_DIR}/../../overlays/istio/generated-manifest.yaml
popd

kubectl create ns istio-system
for i in 1 2 3 ; do kustomize build test/overlays/istio | kubectl apply -f - && break || sleep 15; done

echo "Waiting for Istio to be ready ..."
kubectl wait --for=condition=Ready pods --all --timeout=240s -n istio-system

# Necessary since istio is the default ingressClassName in kserve.yaml
echo "Creating istio ingress class"
cat <<EOF | kubectl apply -f -
apiVersion: networking.k8s.io/v1
kind: IngressClass
metadata:
  name: istio
spec:
  controller: istio.io/ingress-controller
EOF

echo "Installing Knative serving ..."
pushd ${SCRIPT_DIR}/../../overlays/knative >/dev/null
  curl -s -O -L https://github.com/knative/serving/releases/download/${KNATIVE_VERSION}/serving-core.yaml
  curl -s -O -L https://github.com/knative/net-istio/releases/download/${KNATIVE_VERSION}/release.yaml

  # Kustomize does not work with integer map keys
  sed -i 's/8443:/"8443":/g' release.yaml
popd

for i in 1 2 3 ; do kustomize build test/overlays/knative | kubectl apply -f - && break || sleep 15; done

echo "Waiting for Knative to be ready ..."
kubectl wait --for=condition=Ready pods --all --timeout=300s -n knative-serving -l 'app in (activator,autoscaler,autoscaler-hpa,controller,net-istio-controller,net-istio-webhook)'

echo "Add knative hpa..."
# kubectl apply -f https://github.com/knative/serving/releases/download/knative-v1.0.0/serving-hpa.yaml

# Skip tag resolution for certain domains
kubectl patch cm config-deployment --patch '{"data":{"registries-skipping-tag-resolving":"nvcr.io,index.docker.io"}}' -n knative-serving

echo "Installing cert-manager ..."
kubectl create namespace cert-manager
sleep 2
kubectl apply --validate=false -f https://github.com/jetstack/cert-manager/releases/download/${CERT_MANAGER_VERSION}/cert-manager.yaml

echo "Waiting for cert-manager to be ready ..."
kubectl wait --for=condition=ready pod -l 'app in (cert-manager,webhook)' --timeout=180s -n cert-manager
