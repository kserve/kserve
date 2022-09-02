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

# The script is used to deploy knative and kserve, and run e2e tests.

set +e

echo "Starting debug of logger"

pushd test/e2e > /dev/null
    pytest logger/test_logger.py
popd

echo "printing all logs in isvc"
kubectl logs service/isvc-logger-predictor-default-00001-private --all-containers -n kserve-ci-e2e-test
kubectl logs -l component=predictor --all-containers -n kserve-ci-e2e-test 

echo "printing all resources in ci ns"
kubectl get all -n kserve-ci-e2e-test
echo "printing all pods in kserve ns"
kubectl get pods -n kserve

echo "printing isvc yaml"

kubectl get isvc message-dumper -n kserve-ci-e2e-test -o yaml

echo "printing node details"
kubectl describe nodes

echo "printing kubectl pod"
kubectl top pods -A