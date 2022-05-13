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
eksctl get clusters -ojson | jq -r '.[] | .Name'
eksctl delete cluster kubeflow-kserve-presubmit-e2e-1910-0cde85f-2880-ddf1
eksctl delete cluster kubeflow-kserve-presubmit-e2e-1910-24da27a-4000-f5d8
eksctl delete cluster kubeflow-kserve-presubmit-e2e-1910-2a49255-6896-4e03
eksctl delete cluster kubeflow-kserve-presubmit-e2e-1910-3286b70-6656-4913
eksctl delete cluster kubeflow-kserve-presubmit-e2e-1910-5600c53-7632-cddf
eksctl delete cluster kubeflow-kserve-presubmit-e2e-1910-7f5ab9b-7760-8737
eksctl delete cluster kubeflow-kserve-presubmit-e2e-1910-d038166-0240-1bae
eksctl delete cluster kubeflow-kserve-presubmit-e2e-1910-e153849-8368-5a5c
eksctl delete cluster kubeflow-kserve-presubmit-e2e-1910-fb13c0c-8224-1c64
eksctl delete cluster kubeflow-kserve-presubmit-e2e-1910-fc87b0a-2368-09b3
eksctl delete cluster kubeflow-kserve-presubmit-e2e-2082-0868edc-1776-095a
eksctl delete cluster kubeflow-kserve-presubmit-e2e-2082-25e4fcc-8544-9e22
eksctl delete cluster kubeflow-kserve-presubmit-e2e-2082-30b2adc-7440-cc9d
eksctl delete cluster kubeflow-kserve-presubmit-e2e-2082-30b2adc-8784-43c5
eksctl delete cluster kubeflow-kserve-presubmit-e2e-2082-3202cb3-9792-2846
eksctl delete cluster kubeflow-kserve-presubmit-e2e-2082-99f0744-2064-83f8
eksctl delete cluster kubeflow-kserve-presubmit-e2e-2082-a308021-3216-a5d7
eksctl delete cluster kubeflow-kserve-presubmit-e2e-2082-a308021-8368-0dc8
eksctl delete cluster kubeflow-kserve-presubmit-e2e-2082-c5ab86f-5616-9d0f
eksctl delete cluster kubeflow-kserve-presubmit-e2e-2082-dcccbfd-0800-29a4
eksctl delete cluster kubeflow-kserve-presubmit-e2e-2174-9c93fd4-1232-5f7a
eksctl delete cluster kubeflow-kserve-presubmit-e2e-2174-9c93fd4-5072-97ec
eksctl delete cluster kubeflow-kserve-presubmit-e2e-2174-9c93fd4-7664-2cb3
eksctl delete cluster kubeflow-kserve-presubmit-e2e-2178-358fe65-5552-8604
INGRESS_GATEWAY_SERVICE=$(kubectl get svc --namespace istio-system --selector="app=istio-ingressgateway" --output jsonpath='{.items[0].metadata.name}')
kubectl port-forward --namespace istio-system svc/${INGRESS_GATEWAY_SERVICE} 8080:80 &
export KSERVE_INGRESS_HOST_PORT=localhost:8080
echo "Starting E2E functional tests ..."
pushd test/e2e >/dev/null
  pytest predictor/test_sklearn.py
popd
