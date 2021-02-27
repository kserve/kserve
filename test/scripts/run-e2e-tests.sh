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

# The script is used to deploy knative and kfserving, and run e2e tests.

set -o errexit
set -o nounset
set -o pipefail

CLUSTER_NAME="${CLUSTER_NAME}"
AWS_REGION="${AWS_REGION}"

ISTIO_VERSION="1.7.1"
KNATIVE_VERSION="v0.20.0"
KUBECTL_VERSION="v1.20.0"
CERT_MANAGER_VERSION="v1.1.0"

echo "Upgrading kubectl ..."
wget -q -O /usr/local/bin/kubectl https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/amd64/kubectl
chmod a+x /usr/local/bin/kubectl

echo "Configuring kubectl ..."
pip3 install awscli --upgrade --user
aws eks update-kubeconfig --region=${AWS_REGION} --name=${CLUSTER_NAME}

echo "Install istio ..."
mkdir istio_tmp
pushd istio_tmp >/dev/null
  curl -L https://git.io/getLatestIstio | ISTIO_VERSION=${ISTIO_VERSION} sh -
  cd istio-${ISTIO_VERSION}
  export PATH=$PWD/bin:$PATH
  istioctl operator init
  cat << EOF > ./istio-minimal-operator.yaml
apiVersion: install.istio.io/v1alpha1
kind: IstioOperator
spec:
  values:
    global:
      proxy:
        autoInject: disabled
      useMCP: false
      # The third-party-jwt is not enabled on all k8s.
      # See: https://istio.io/docs/ops/best-practices/security/#configure-third-party-service-account-tokens
      jwtPolicy: first-party-jwt

  meshConfig:
    accessLogFile: /dev/stdout

  addonComponents:
    pilot:
      enabled: true

  components:
    ingressGateways:
      - name: istio-ingressgateway
        enabled: true
EOF
  istioctl manifest install -f ./istio-minimal-operator.yaml

popd

echo "Waiting for istio started ..."
kubectl wait --for=condition=Ready pods --all --timeout=180s -n istio-system

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
kubectl wait --for=condition=Ready pods --all --timeout=180s -n knative-serving -l 'app in (activator,autoscaler,autoscaler-hpa,controller,istio-webhook,networking-istio)'

echo "Installing cert manager ..."
kubectl create namespace cert-manager
sleep 2
kubectl apply --validate=false -f https://github.com/jetstack/cert-manager/releases/download/${CERT_MANAGER_VERSION}/cert-manager.yaml

echo "Waiting for cert manager started ..."
kubectl wait --for=condition=ready pod -l app=cert-manager --timeout=180s -n cert-manager

echo "Install KFServing ..."
export GOPATH="$HOME/go"
export PATH="${PATH}:${GOPATH}/bin"
mkdir -p ${GOPATH}/src/github.com/kubeflow
cp -rf ../kfserving ${GOPATH}/src/github.com/kubeflow
cd ${GOPATH}/src/github.com/kubeflow/kfserving

wget -O $GOPATH/bin/yq https://github.com/mikefarah/yq/releases/download/3.3.2/yq_linux_amd64
chmod +x $GOPATH/bin/yq
sed -i -e "s/latest/${PULL_BASE_SHA}/g" config/overlays/test/configmap/inferenceservice.yaml
sed -i -e "s/latest/${PULL_BASE_SHA}/g" config/overlays/test/manager_image_patch.yaml
make deploy-ci

echo "Waiting for KFServing started ..."
kubectl wait --for=condition=Ready pods --all --timeout=180s -n kfserving-system

echo "Creating a namespace kfserving-ci-test ..."
kubectl create namespace kfserving-ci-e2e-test

echo "Istio, Knative and KFServing have been installed and started."

echo "Installing KFServing Python SDK ..."
python3 -m pip install --upgrade pip
pip3 install pytest==6.0.2 pytest-xdist pytest-rerunfailures
pip3 install --upgrade pytest-tornasync
pip3 install urllib3==1.24.2
pip3 install --upgrade setuptools
pushd python/kfserving >/dev/null
    pip3 install -r requirements.txt
    python3 setup.py install --force --user
popd

echo "Starting E2E functional tests ..."
pushd test/e2e >/dev/null
  pytest -n 4 --ignore=credentials/test_set_creds.py
popd
