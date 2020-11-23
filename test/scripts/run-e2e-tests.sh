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

ISTIO_VERSION="1.3.1"
KNATIVE_VERSION="v0.15.0"
KUBECTL_VERSION="v1.14.0"
CERT_MANAGER_VERSION="v0.12.0"
# Check and wait for istio/knative/kfserving pod started normally.
waiting_pod_running(){
    namespace=$1
    TIMEOUT=180
    PODNUM=$(kubectl get deployments -n ${namespace} | grep -v NAME | wc -l)
    until kubectl get pods -n ${namespace} | grep -E "Running" | [[ $(wc -l) -eq $PODNUM ]]; do
        echo Pod Status $(kubectl get pods -n ${namespace} | grep -E "Running" | wc -l)/$PODNUM

        sleep 10
        TIMEOUT=$(( TIMEOUT - 10 ))
        if [[ $TIMEOUT -eq 0 ]];then
            echo "Timeout to waiting for pod start."
            kubectl get pods -n ${namespace}
            exit 1
        fi
    done
}

echo "Upgrading kubectl ..."
# The kubectl need to be upgraded to 1.14.0 to avoid dismatch issue.
wget -q -O /usr/local/bin/kubectl https://storage.googleapis.com/kubernetes-release/release/${KUBECTL_VERSION}/bin/linux/amd64/kubectl
chmod a+x /usr/local/bin/kubectl

echo "Configuring kubectl ..."
pip3 install awscli --upgrade --user
aws eks update-kubeconfig --region=${AWS_REGION} --name=${CLUSTER_NAME}

# Install and Initialize Helm
wget https://get.helm.sh/helm-v3.0.2-linux-amd64.tar.gz
tar xvf helm-v3.0.2-linux-amd64.tar.gz
mv linux-amd64/helm /usr/local/bin/

echo "Install istio ..."
mkdir istio_tmp
pushd istio_tmp >/dev/null
  curl -L https://git.io/getLatestIstio | ISTIO_VERSION=${ISTIO_VERSION} sh -
  cd istio-${ISTIO_VERSION}
  export PATH=$PWD/bin:$PATH
  kubectl create namespace istio-system
  #install istio lean
  helm template --namespace=istio-system \
  --set prometheus.enabled=false \
  --set mixer.enabled=false \
  --set mixer.policy.enabled=false \
  --set mixer.telemetry.enabled=false \
  `# Pilot doesn't need a sidecar.` \
  --set pilot.sidecar=false \
  --set pilot.resources.requests.memory=128Mi \
  `# Disable galley (and things requiring galley).` \
  --set galley.enabled=false \
  --set global.useMCP=false \
  `# Disable security / policy.` \
  --set security.enabled=false \
  --set global.disablePolicyChecks=true \
  `# Disable sidecar injection.` \
  --set sidecarInjectorWebhook.enabled=false \
  --set global.proxy.autoInject=disabled \
  --set global.omitSidecarInjectorConfigMap=true \
  --set gateways.istio-ingressgateway.autoscaleMin=1 \
  --set gateways.istio-ingressgateway.autoscaleMax=2 \
  `# Set pilot trace sampling to 100%` \
  --set pilot.traceSampling=100 \
  --set global.mtls.auto=false \
  install/kubernetes/helm/istio \
  > ./istio-lean.yaml

  kubectl apply -f istio-lean.yaml

  #use cluster local gateway
  helm template --namespace=istio-system \
  --set gateways.custom-gateway.autoscaleMin=1 \
  --set gateways.custom-gateway.autoscaleMax=1 \
  --set gateways.custom-gateway.cpu.targetAverageUtilization=60 \
  --set gateways.custom-gateway.labels.app='cluster-local-gateway' \
  --set gateways.custom-gateway.labels.istio='cluster-local-gateway' \
  --set gateways.custom-gateway.type='ClusterIP' \
  --set gateways.istio-ingressgateway.enabled=false \
  --set gateways.istio-egressgateway.enabled=false \
  --set gateways.istio-ilbgateway.enabled=false \
  install/kubernetes/helm/istio \
  -f install/kubernetes/helm/istio/example-values/values-istio-gateways.yaml \
  | sed -e "s/custom-gateway/cluster-local-gateway/g" -e "s/customgateway/clusterlocalgateway/g" \
  > ./istio-local-gateway.yaml

  kubectl apply -f istio-local-gateway.yaml
popd

echo "Waiting for istio started ..."
waiting_pod_running "istio-system"

echo "Installing knative serving ..."
kubectl apply --filename https://github.com/knative/serving/releases/download/${KNATIVE_VERSION}/serving-crds.yaml
kubectl apply --filename https://github.com/knative/serving/releases/download/${KNATIVE_VERSION}/serving-core.yaml
kubectl apply --filename https://github.com/knative/net-istio/releases/download/${KNATIVE_VERSION}/release.yaml

echo "Waiting for knative started ..."
waiting_pod_running "knative-serving"
# skip nvcr.io for tag resolution due to auth issue
kubectl patch cm config-deployment --patch '{"data":{"registriesSkippingTagResolving":"nvcr.io"}}' -n knative-serving
# give longer revision timeout
kubectl patch cm config-deployment --patch '{"data":{"progressDeadline": "600s"}}' -n knative-serving
echo "Installing cert manager ..."
kubectl create namespace cert-manager
sleep 2
kubectl apply --validate=false -f https://github.com/jetstack/cert-manager/releases/download/${CERT_MANAGER_VERSION}/cert-manager.yaml

echo "Waiting for cert manager started ..."
kubectl wait --for=condition=ready pod -l app=cert-manager -n cert-manager

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
kubectl wait --for=condition=ready pod -l control-plane=kfserving-controller-manager -n kfserving-system

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
  pytest -n 3 --ignore=credentials/test_set_creds.py
popd
