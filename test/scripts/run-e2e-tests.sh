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

echo "Checking istio ..."
waiting_pod_running "istio-system"

echo "Installing knative serving ..."
kubectl apply --selector knative.dev/crd-install=true --filename https://github.com/knative/serving/releases/download/${KNATIVE_VERSION}/serving.yaml
sleep 2
kubectl apply --filename https://github.com/knative/serving/releases/download/${KNATIVE_VERSION}/serving.yaml

echo "Checking knative ..."
waiting_pod_running "knative-serving"

echo "Install KFServing ..."
export GOPATH="$HOME/go"
export PATH="${PATH}:${GOPATH}/bin"
mkdir -p ${GOPATH}/src/github.com/kubeflow
cp -rf ../kfserving ${GOPATH}/src/github.com/kubeflow
cd ${GOPATH}/src/github.com/kubeflow/kfserving

make deploy-test

echo "Checking kfserving ..."
sleep 20
kubectl get pods -n kfserving-system
waiting_pod_running "kfserving-system"

echo "KFServing has been installed and started."

echo "Create a kfservice ..."
sleep 60
kubectl create namespace kfserving-ci-test
kubectl create -f test/scripts/tensorflow.yaml -n kfserving-ci-test

echo "Checking the kfservice status ..."
sleep 20
waiting_pod_running "kfserving-ci-test"
