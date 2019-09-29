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
KNATIVE_VERSION="v0.9.0"

# Check and wait for istio/knative/kfserving pod started normally.
waiting_pod_running(){
    namespace=$1
    TIMEOUT=120
    PODNUM=$(kubectl get pods -n ${namespace} | grep -v NAME | wc -l)
    until kubectl get pods -n ${namespace} | grep -E "Running|Completed" | [[ $(wc -l) -eq $PODNUM ]]; do
        echo Pod Status $(kubectl get pods -n ${namespace} | grep -E "Running|Completed" | wc -l)/$PODNUM

        sleep 10
        TIMEOUT=$(( TIMEOUT - 1 ))
        if [[ $TIMEOUT -eq 0 ]];then
            echo "Timeout to waiting for pod start."
            kubectl get pods -n ${namespace}
            exit 1
        fi
    done
}

echo "Activating service-account ..."
gcloud auth activate-service-account --key-file=${GOOGLE_APPLICATION_CREDENTIALS}

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
  curl -L https://git.io/getLatestIstio | sh -
  cd istio-*
  export PATH=$PWD/bin:$PATH
  kubectl create namespace istio-system
  helm template install/kubernetes/helm/istio-init \
  --name istio-init --namespace istio-system | kubectl apply -f -
  sleep 30
  helm template install/kubernetes/helm/istio \
  --name istio --namespace istio-system | kubectl apply -f -
popd

echo "Waiting for istio started ..."
sleep 15
waiting_pod_running "istio-system"

echo "Installing knative serving ..."
kubectl apply --selector knative.dev/crd-install=true --filename https://github.com/knative/serving/releases/download/${KNATIVE_VERSION}/serving.yaml
sleep 2
kubectl apply --filename https://github.com/knative/serving/releases/download/${KNATIVE_VERSION}/serving.yaml

echo "Waiting for knative started ..."
waiting_pod_running "knative-serving"

echo "Install KFServing ..."
export GOPATH="$HOME/go"
export PATH="${PATH}:${GOPATH}/bin"
mkdir -p ${GOPATH}/src/github.com/kubeflow
cp -rf ../kfserving ${GOPATH}/src/github.com/kubeflow
cd ${GOPATH}/src/github.com/kubeflow/kfserving
make deploy-test

echo "Waiting for KFServing started ..."
sleep 20
waiting_pod_running "kfserving-system"
sleep 60  # Wait for webhook install finished totally.

echo "Creating a namespace kfserving-ci-test ..."
kubectl create namespace kfserving-ci-e2e-test

echo "Istio, Knative and KFServing have been installed and started."

echo "Upgrading Python to 3.6 to install KFServing SDK ..."
apt-get update -yqq
apt-get install -yqq --no-install-recommends software-properties-common
add-apt-repository -y ppa:jonathonf/python-3.6
apt-get update -yqq
apt-get install -yqq --no-install-recommends  python3.6 python3-pip
update-alternatives --install /usr/bin/python3 python3 /usr/bin/python3.5 1
update-alternatives --install /usr/bin/python3 python3 /usr/bin/python3.6 2

echo "Installing KFServing Python SDK ..."
python3 -m pip install --upgrade pip
pip3 install --upgrade pytest
pip3 install --upgrade pytest-tornasync
pip3 install urllib3==1.24.2
pushd python/kfserving >/dev/null
    pip3 install -r requirements.txt
    python3 setup.py install --force
popd

echo "Starting E2E functional tests ..."
pushd test/e2e >/dev/null
  pytest
popd
