#!/usr/bin/env bash

# Copyright 2018 The Knative Authors
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

set -o errexit

source $(dirname $0)/../vendor/github.com/knative/test-infra/scripts/library.sh

: ${PROJECT_ID:="knative-environments"}
readonly PROJECT_ID
readonly K8S_CLUSTER_NAME=${1:?"First argument must be the kubernetes cluster name."}
readonly K8S_CLUSTER_VERSION=latest
readonly K8S_CLUSTER_REGION=us-central1
readonly K8S_CLUSTER_ZONE=a
readonly K8S_CLUSTER_MACHINE=n1-standard-8
readonly K8S_CLUSTER_NODES=5
readonly PROJECT_USER=$(gcloud config get-value core/account)
readonly CURRENT_PROJECT=$(gcloud config get-value project)

function cleanup() {
  gcloud config set project ${CURRENT_PROJECT}
}

cd ${REPO_ROOT_DIR}
trap cleanup EXIT

echo "Using project ${PROJECT_ID} and user ${PROJECT_USER}"
gcloud config set project ${PROJECT_ID}

existing_cluster=$(gcloud container clusters list \
    --zone=${K8S_CLUSTER_REGION}-${K8S_CLUSTER_ZONE} --filter="name=${K8S_CLUSTER_NAME}")
if [[ -n "${existing_cluster}" ]]; then
  header "Deleting previous cluster ${K8S_CLUSTER_NAME} in ${PROJECT_ID}"
  gcloud -q container clusters delete \
    --zone=${K8S_CLUSTER_REGION}-${K8S_CLUSTER_ZONE} ${K8S_CLUSTER_NAME}
fi

header "Creating cluster ${K8S_CLUSTER_NAME} in ${PROJECT_ID}"
gcloud --project=${PROJECT_ID} container clusters create \
  --enable-basic-auth \
  --no-issue-client-certificate \
  --no-enable-autoupgrade \
  --cluster-version=${K8S_CLUSTER_VERSION} \
  --image-type=${SERVING_GKE_IMAGE} \
  --zone=${K8S_CLUSTER_REGION}-${K8S_CLUSTER_ZONE} \
  --scopes=cloud-platform \
  --machine-type=${K8S_CLUSTER_MACHINE} \
  --enable-autoscaling \
  --min-nodes=1 \
  --max-nodes=${K8S_CLUSTER_NODES} \
  ${K8S_CLUSTER_NAME}

header "Setting cluster admin"
acquire_cluster_admin_role ${PROJECT_USER} ${K8S_CLUSTER_NAME} ${K8S_CLUSTER_REGION} ${K8S_CLUSTER_ZONE}

kubectl config set-context $(kubectl config current-context) --namespace=default
start_latest_knative_serving

header "Knative Serving deployed successfully to ${K8S_CLUSTER_NAME}"
