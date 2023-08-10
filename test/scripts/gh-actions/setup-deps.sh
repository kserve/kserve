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

ISTIO_VERSION="1.17.2"
KNATIVE_VERSION="1.10.2"
CERT_MANAGER_VERSION="v1.5.0"
YQ_VERSION="v4.28.1"

echo "Installing yq ..."
wget https://github.com/mikefarah/yq/releases/download/${YQ_VERSION}/yq_linux_amd64 -O /usr/local/bin/yq && chmod +x /usr/local/bin/yq

echo "Installing Istio ..."
mkdir istio_tmp
pushd istio_tmp >/dev/null
  curl -L https://istio.io/downloadIstio | ISTIO_VERSION=${ISTIO_VERSION} sh -
  cd istio-${ISTIO_VERSION}
  export PATH=$PWD/bin:$PATH
  istioctl manifest generate --set meshConfig.accessLogFile=/dev/stdout > ${SCRIPT_DIR}/../../overlays/istio/generated-manifest.yaml
popd

kubectl create ns istio-system
for i in 1 2 3 ; do kubectl apply -k test/overlays/istio && break || sleep 15; done

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

source  ./test/scripts/gh-actions/install-knative-operator.sh

echo "Installing Knative serving ..."
for i in 1 2 3; do
  kn operator install --component serving -n knative-serving --istio -v "${KNATIVE_VERSION}" && break || sleep 15
done

# configure resources
kn operator configure resources --component serving --deployName controller --container controller --requestCPU 5m --requestMemory 32Mi --limitCPU 100m --limitMemory 128Mi -n knative-serving
kn operator configure resources --component serving --deployName activator --container activator --requestCPU 5m --requestMemory 32Mi --limitCPU 100m --limitMemory 128Mi -n knative-serving
kn operator configure resources --component serving --deployName autoscaler --container autoscaler --requestCPU 5m --requestMemory 32Mi --limitCPU 100m --limitMemory 128Mi -n knative-serving
kn operator configure resources --component serving --deployName domain-mapping --container domain-mapping --requestCPU 5m --requestMemory 32Mi --limitCPU 100m --limitMemory 128Mi -n knative-serving
kn operator configure resources --component serving --deployName webhook --container webhook --requestCPU 5m --requestMemory 32Mi --limitCPU 100m --limitMemory 128Mi -n knative-serving
kn operator configure resources --component serving --deployName domainmapping-webhook --container domainmapping-webhook --requestCPU 5m --requestMemory 32Mi --limitCPU 100m --limitMemory 128Mi -n knative-serving
kn operator configure resources --component serving --deployName net-istio-controller --container controller --requestCPU 5m --requestMemory 32Mi --limitCPU 100m --limitMemory 128Mi -n knative-serving
kn operator configure resources --component serving --deployName net-istio-webhook --container webhook --requestCPU 5m --requestMemory 32Mi --limitCPU 100m --limitMemory 128Mi -n knative-serving

echo "Waiting for Knative to be ready ..."
kubectl wait --for=condition=Ready pods --all --timeout=400s -n knative-serving -l 'app in (webhook, activator,autoscaler,autoscaler-hpa,controller,net-istio-controller,net-istio-webhook)'

# echo "Add knative hpa..."
# kubectl apply -f https://github.com/knative/serving/releases/download/knative-v1.0.0/serving-hpa.yaml

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

echo "Installing cert-manager ..."
kubectl create namespace cert-manager
sleep 2
kubectl apply --validate=false -f https://github.com/jetstack/cert-manager/releases/download/${CERT_MANAGER_VERSION}/cert-manager.yaml

echo "Waiting for cert-manager to be ready ..."
kubectl wait --for=condition=ready pod -l 'app in (cert-manager,webhook)' --timeout=180s -n cert-manager
