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

# This script runs the end-to-end tests against Knative Serving built from source.
# It is started by prow for each PR. For convenience, it can also be executed manually.

# If you already have a Knative cluster setup and kubectl pointing
# to it, call this script with the --run-tests arguments and it will use
# the cluster and run the tests.

# Calling this script without arguments will create a new cluster in
# project $PROJECT_ID, start knative in it, run the tests and delete the
# cluster.

source $(dirname $0)/e2e-common.sh

# Helper functions.

function dump_extra_cluster_state() {
  echo ">>> Routes:"
  kubectl get routes -o yaml --all-namespaces
  echo ">>> Configurations:"
  kubectl get configurations -o yaml --all-namespaces
  echo ">>> Revisions:"
  kubectl get revisions -o yaml --all-namespaces

  for app in controller webhook autoscaler activator; do
    dump_app_logs ${app} knative-serving
  done
}

function knative_setup() {
  install_knative_serving
}

# Script entry point.

initialize $@

# Run the tests
header "Running tests"

failed=0

# Run conformance and e2e tests.
go_test_e2e -timeout=30m ./test/conformance ./test/e2e \
  --resolvabledomain=$(use_resolvable_domain) || failed=1

# Run scale tests.
go_test_e2e -timeout=10m ./test/scale || failed=1

# Require that both set of tests succeeded.
(( failed )) && fail_test

success
