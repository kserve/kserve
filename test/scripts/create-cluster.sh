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

# This shell script is used to build a cluster and create a namespace.

set -o errexit
set -o nounset
set -o pipefail

CLUSTER_NAME="${CLUSTER_NAME}"
ZONE="${GCP_ZONE}"
PROJECT="${GCP_PROJECT}"
NAMESPACE="${DEPLOY_NAMESPACE}"

echo "Activating service-account ..."
gcloud auth activate-service-account --key-file=${GOOGLE_APPLICATION_CREDENTIALS}

echo "Creating cluster ${CLUSTER_NAME} ... "
gcloud --project ${PROJECT} beta container clusters create ${CLUSTER_NAME} \
    --addons=HorizontalPodAutoscaling,HttpLoadBalancing \
    --machine-type=n1-standard-4 \
    --cluster-version 1.13 --zone ${ZONE} \
    --enable-stackdriver-kubernetes --enable-ip-alias \
    --enable-autoscaling --min-nodes=3 --max-nodes=10 \
    --enable-autorepair \
    --scopes cloud-platform

echo "Configuring kubectl ..."
gcloud --project ${PROJECT} container clusters get-credentials ${CLUSTER_NAME} --zone ${ZONE}

echo "Creating namespace ${NAMESPACE} ..."
kubectl create namespace ${NAMESPACE}
