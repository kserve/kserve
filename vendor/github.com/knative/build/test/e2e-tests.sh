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

# This script runs the end-to-end tests against the build controller
# built from source. It is started by prow for each PR.
# For convenience, it can also be executed manually.

# If you already have the *_OVERRIDE environment variables set, call
# this script with the --run-tests arguments and it will use the cluster
# and run the tests.

# Calling this script without arguments will create a new cluster in
# project $PROJECT_ID, start the controller, run the tests and delete
# the cluster.

source $(dirname $0)/e2e-common.sh

# Helper functions.

function dump_extra_cluster_state() {
  echo ">>> Builds:"
  kubectl get builds -o yaml --all-namespaces
  echo ">>> Pods:"
  kubectl get pods -o yaml --all-namespaces

  dump_app_logs controller knative-build
}

# Script entry point.

initialize $@

# Run the tests

failed=0

header "Running Go e2e tests"
go_test_e2e ./test/e2e/... || failed=1

run_yaml_tests || failed=1

(( failed )) && fail_test
success
