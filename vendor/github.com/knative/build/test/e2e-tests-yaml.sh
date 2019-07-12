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

# This script runs the YAML end-to-end tests against the build controller
# built from source. It is not run by prow for each PR, see e2e-tests.sh for that.

source $(dirname $0)/e2e-common.sh

# Script entry point.

header "Setting up environment"

knative_setup

# Run the tests

run_yaml_tests || fail_test
success
