#!/bin/bash

# Copyright 2021 The KServe Authors.
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

set -o errexit
set -o nounset
set -o pipefail

CLUSTER_NAME="${CLUSTER_NAME}"
AWS_REGION="${AWS_REGION}"

ISTIO_VERSION="1.12.0"
KNATIVE_VERSION="knative-v1.0.0"
KUBECTL_VERSION="v1.20.2"
CERT_MANAGER_VERSION="v1.2.0"

echo "Upgrading kubectl ..."
wget -q -O /usr/local/bin/kubectl https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/amd64/kubectl
chmod a+x /usr/local/bin/kubectl

echo "Configuring kubectl ..."
pip3 install awscli --upgrade --user
aws eks update-kubeconfig --region=${AWS_REGION} --name=${CLUSTER_NAME}

echo "Updating kustomize"
KUSTOMIZE_PATH=$(which kustomize)
rm -rf ${KUSTOMIZE_PATH}
curl -s "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh" | bash -s -- 4.2.0 ${KUSTOMIZE_PATH::-10}


echo "Install istio ..."
mkdir istio_tmp
pushd istio_tmp >/dev/null
  curl -L https://istio.io/downloadIstio | ISTIO_VERSION=${ISTIO_VERSION} sh -
  cd istio-${ISTIO_VERSION}
  export PATH=$PWD/bin:$PATH
  istioctl install --set meshConfig.accessLogFile=/dev/stdout -y
popd

echo "Waiting for istio started ..."
kubectl wait --for=condition=Ready pods --all --timeout=180s -n istio-system

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

echo "Installing knative serving ..."
kubectl apply -f https://github.com/knative/operator/releases/download/${KNATIVE_VERSION}/operator.yaml
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Namespace
metadata:
 name: knative-serving
 labels:
   istio-injection: enabled
---
apiVersion: operator.knative.dev/v1alpha1
kind: KnativeServing
metadata:
  name: knative-serving
  namespace: knative-serving
EOF

echo "Waiting for knative started ..."
kubectl wait --for=condition=Ready knativeservings -n knative-serving knative-serving --timeout=180s
kubectl wait --for=condition=Ready pods --all --timeout=180s -n knative-serving -l 'app in (activator,autoscaler,autoscaler-hpa,controller,net-istio-controller,net-istio-webhook)'

# skip nvcr.io for tag resolution due to auth issue
kubectl patch cm config-deployment --patch '{"data":{"registriesSkippingTagResolving":"nvcr.io"}}' -n knative-serving
# give longer revision timeout
kubectl patch cm config-deployment --patch '{"data":{"progressDeadline": "600s"}}' -n knative-serving

echo "Installing cert manager ..."
kubectl create namespace cert-manager
sleep 2
kubectl apply --validate=false -f https://github.com/jetstack/cert-manager/releases/download/${CERT_MANAGER_VERSION}/cert-manager.yaml

echo "Waiting for cert manager started ..."
kubectl wait --for=condition=ready pod -l 'app in (cert-manager,webhook)' --timeout=180s -n cert-manager

echo "Install KServe ..."
export GOPATH="$HOME/go"
export PATH="${PATH}:${GOPATH}/bin"

wget -O $GOPATH/bin/yq https://github.com/mikefarah/yq/releases/download/3.3.2/yq_linux_amd64
chmod +x $GOPATH/bin/yq
sed -i -e "s/latest/${PULL_BASE_SHA}/g" config/overlays/test/configmap/inferenceservice.yaml
sed -i -e "s/latest/${PULL_BASE_SHA}/g" config/overlays/test/runtimes/kustomization.yaml
sed -i -e "s/latest/${PULL_BASE_SHA}/g" config/overlays/test/manager_image_patch.yaml
make deploy-ci

echo "Waiting for KServe started ..."
kubectl wait --for=condition=Ready pods --all --timeout=180s -n kserve
kubectl get events -A

echo "Add testing models to minio stroage ..."
kubectl apply -f config/overlays/test/minio/minio-init-job.yaml -n kserve
kubectl wait --for=condition=complete --timeout=30s job/minio-init -n kserve

echo "Creating a namespace kserve-ci-test ..."
kubectl create namespace kserve-ci-e2e-test

echo "Add storageSpec testing secrets ..."
kubectl apply -f config/overlays/test/minio/minio-user-secret.yaml -n kserve-ci-e2e-test

echo "Istio, Knative and KServe have been installed and started."

echo "Installing KServe Python SDK ..."
python3 -m pip install --upgrade pip
pushd python/kserve >/dev/null
    pip3 install -e .[test] --user
popd

echo "Starting E2E functional tests ..."
pushd test/e2e >/dev/null
  pytest -n 4 --ignore=credentials/test_set_creds.py
popd
