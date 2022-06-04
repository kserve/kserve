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

# This shell script is used to build a cluster and create a namespace.

set -o errexit
set -o nounset
set -o pipefail

EKS_CLUSTER_NAME="${CLUSTER_NAME}"
DESIRED_NODE="${DESIRED_NODE:-4}"
MIN_NODE="${MIN_NODE:-1}"
MAX_NODE="${MAX_NODE:-4}"

echo "Starting to create eks cluster"
eksctl create cluster \
	--name ${EKS_CLUSTER_NAME} \
	--version 1.21 \
	--region us-west-2 \
	--zones us-west-2a,us-west-2b,us-west-2c \
	--nodegroup-name linux-nodes \
	--node-type m5.xlarge \
	--nodes ${DESIRED_NODE} \
	--nodes-min ${MIN_NODE} \
	--nodes-max ${MAX_NODE}
echo "Successfully create eks cluster ${EKS_CLUSTER_NAME}"
