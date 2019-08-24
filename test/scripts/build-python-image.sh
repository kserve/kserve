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

# This shell script is used to build an image from our argo workflow

set -o errexit
set -o nounset
set -o pipefail

cd python
if [ ! $# -eq 2 ]; then
  echo "build-python-image.sh dockerFile imageName"
  exit -1
fi
if [ ! -f $1 ]; then
  echo "dockerFile $1 doesn't exist"
  exit -1
fi

REGISTRY="${GCP_REGISTRY}"
PROJECT="${GCP_PROJECT}"
VERSION=$(git describe --tags --always --dirty)

echo "Activating service-account"
gcloud auth activate-service-account --key-file=${GOOGLE_APPLICATION_CREDENTIALS}

cp $1 Dockerfile
gcloud builds submit . --tag=${REGISTRY}/${REPO_NAME}/$2:${VERSION} --project=${PROJECT} --timeout=20m
gcloud container images add-tag --quiet ${REGISTRY}/${REPO_NAME}/$2:${VERSION} ${REGISTRY}/${REPO_NAME}/$2:latest --verbosity=info
