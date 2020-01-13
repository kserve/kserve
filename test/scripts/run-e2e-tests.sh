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
ZONE="${GCP_ZONE}"
PROJECT="${GCP_PROJECT}"
NAMESPACE="${DEPLOY_NAMESPACE}"
REGISTRY="${GCP_REGISTRY}"
ISTIO_VERSION="1.3.1"
KNATIVE_VERSION="v0.10.0"
KUBECTL_VERSION="v1.14.0"
CERT_MANAGER_VERSION="v0.12.0"
# Check and wait for istio/knative/kfserving pod started normally.
waiting_pod_running(){
    namespace=$1
    TIMEOUT=120
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

waiting_for_kfserving_controller(){
    TIMEOUT=120
    until [[ $(kubectl get statefulsets kfserving-controller-manager -n kfserving-system -o=jsonpath='{.status.readyReplicas}') -eq 1 ]]; do
        kubectl get pods -n kfserving-system
        kubectl get cm -n kfserving-system
        sleep 10
        TIMEOUT=$(( TIMEOUT - 10 ))
        if [[ $TIMEOUT -eq 0 ]];then
            echo "Timeout to waiting for kfserving controller to start."
            kubectl get pods -n kfserving-system
            exit 1
        fi
    done
}

echo "Activating service-account ..."
gcloud auth activate-service-account --key-file=${GOOGLE_APPLICATION_CREDENTIALS}

echo "Upgrading kubectl ..."
# The kubectl need to be upgraded to 1.14.0 to avoid dismatch issue.
wget -q -O /usr/local/bin/kubectl https://storage.googleapis.com/kubernetes-release/release/${KUBECTL_VERSION}/bin/linux/amd64/kubectl
chmod a+x /usr/local/bin/kubectl

echo "Configuring kubectl ..."
gcloud --project ${PROJECT} container clusters get-credentials ${CLUSTER_NAME} --zone ${ZONE}
kubectl config set-context $(kubectl config current-context) --namespace=default

echo "Grant cluster-admin permissions to the current user ..."
kubectl create clusterrolebinding cluster-admin-binding \
  --clusterrole=cluster-admin \
  --user=$(gcloud config get-value core/account)

echo "Install istio ..."
mkdir istio_tmp
pushd istio_tmp >/dev/null
  curl -L https://git.io/getLatestIstio | ISTIO_VERSION=${ISTIO_VERSION} sh -
  cd istio-${ISTIO_VERSION}
  export PATH=$PWD/bin:$PATH
  kubectl create namespace istio-system
  helm template install/kubernetes/helm/istio-init \
  --name istio-init --namespace istio-system | kubectl apply -f -
  sleep 30
  helm template install/kubernetes/helm/istio \
  --name istio --namespace istio-system | kubectl apply -f -

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
kubectl apply --selector knative.dev/crd-install=true --filename https://github.com/knative/serving/releases/download/${KNATIVE_VERSION}/serving.yaml
sleep 2
kubectl apply --filename https://github.com/knative/serving/releases/download/${KNATIVE_VERSION}/serving.yaml

echo "Waiting for knative started ..."
waiting_pod_running "knative-serving"

echo "Installing cert manager ..."
kubectl create namespace cert-manager
sleep 2
kubectl apply --validate=false -f https://github.com/jetstack/cert-manager/releases/download/${CERT_MANAGER_VERSION}/cert-manager.yaml

echo "Waiting for cert manager started ..."
waiting_pod_running "cert-manager"
sleep 30  # Wait for webhook install finished totally.

echo "Install KFServing ..."
export GOPATH="$HOME/go"
export PATH="${PATH}:${GOPATH}/bin"
mkdir -p ${GOPATH}/src/github.com/kubeflow
cp -rf ../kfserving ${GOPATH}/src/github.com/kubeflow
cd ${GOPATH}/src/github.com/kubeflow/kfserving
make deploy-ci

echo "Waiting for KFServing started ..."
waiting_for_kfserving_controller
sleep 30  # Wait for webhook install finished totally.

echo "Creating a namespace kfserving-ci-test ..."
kubectl create namespace kfserving-ci-e2e-test

echo "Istio, Knative and KFServing have been installed and started."

echo "Upgrading Python to 3.6 to install KFServing SDK ..."
apt-get update -yqq
apt-get install -y build-essential checkinstall >/dev/null
apt-get install -y libreadline-gplv2-dev libncursesw5-dev libssl-dev libsqlite3-dev tk-dev libgdbm-dev libc6-dev libbz2-dev >/dev/null
wget https://www.python.org/ftp/python/3.6.9/Python-3.6.9.tar.xz >/dev/null
tar xvf Python-3.6.9.tar.xz >/dev/null
pushd Python-3.6.9  >/dev/null
  ./configure >/dev/null
  make altinstall >/dev/null
popd

update-alternatives --install /usr/bin/python3 python3 /usr/bin/python3.5 1
update-alternatives --install /usr/bin/python3 python3 /usr/local/bin/python3.6 2
# Work around the issue https://github.com/pypa/pip/issues/4924
mv /usr/bin/lsb_release /usr/bin/lsb_release.bak

echo "Installing KFServing Python SDK ..."
python3 -m pip install --upgrade pip
pip3 install --upgrade pytest pytest-xdist
pip3 install --upgrade pytest-tornasync
pip3 install urllib3==1.24.2
pushd python/kfserving >/dev/null
    pip3 install -r requirements.txt
    python3 setup.py install --force
popd

echo "Starting E2E functional tests ..."
pushd test/e2e >/dev/null
  pytest -n 1 
popd
