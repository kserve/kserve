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

# The script will install KServe dependencies in the GH Actions environment.
# (Istio, Knative, cert-manager, kustomize, yq)

set -o errexit
set -o nounset
set -o pipefail

make deploy-ci

echo "Waiting for KServe started ..."
kubectl wait --for=condition=Ready pods --all --timeout=180s -n kserve
kubectl get events -A

echo "Add testing models to minio storage ..."
kubectl apply -f config/overlays/test/minio/minio-init-job.yaml -n kserve
kubectl wait --for=condition=complete --timeout=90s job/minio-init -n kserve

echo "Creating a namespace kserve-ci-test ..."
kubectl create namespace kserve-ci-e2e-test

echo "Add storageSpec testing secrets ..."
kubectl apply -f config/overlays/test/minio/minio-user-secret.yaml -n kserve-ci-e2e-test

echo "Installing Poetry"
export POETRY_VERSION=1.4.0
export POETRY_HOME=/opt/poetry
python3 -m venv $POETRY_HOME && $POETRY_HOME/bin/pip install poetry==$POETRY_VERSION
export PATH="$PATH:$POETRY_HOME/bin"

echo "Installing KServe Python SDK ..."
pushd python/kserve >/dev/null
    poetry config virtualenvs.in-project true
    poetry version $(cat ../VERSION) && poetry install --with=test --no-interaction
popd
