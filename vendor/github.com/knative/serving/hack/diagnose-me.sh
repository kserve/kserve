#!/usr/bin/env bash

# Copyright 2018 The Knative Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -e

CLUSTER_VERSION=1.10

tests=(
  cluster_version
  webhook_running
  webhook_configuration
  controllers_running
  istio_installed
)

test_command() {
  local output="fail"
  local cmd="$1"
  local result="$2"

  if [[ "$(eval "$cmd" 2>/dev/null)" == "$result" ]]; then
    output="ok"
  fi
  printf "\t\t[$output]\n"
}

run_test() {
  local test_name="$1"

  case $test_name in
    cluster_version)
      printf "K8s cluster is running $CLUSTER_VERSION"
      test_cmd='kubectl version --short | grep -i server | grep v$CLUSTER_VERSION >/dev/null && echo $?'
      expected_result=0
      test_command "$test_cmd" "$expected_result"
      ;;
    webhook_running)
      printf "Knative Serving webhook is installed"
      test_cmd="kubectl get pods -n knative-serving -l app=webhook -o jsonpath={.items[].status.phase}"
      expected_result="Running"
      test_command "$test_cmd" "$expected_result"
      ;;
    webhook_configuration)
      printf "Checking webhook configuration...\n"
      object=(configuration routes revisions)
      for obj in ${object[*]}; do
        printf "* configured with $obj resource"
        test_cmd="kubectl get mutatingwebhookconfiguration webhook.knative.dev -o jsonpath={.webhooks[].rules[].resources} | grep $obj >/dev/null && echo $?"
        expected_result=0
        test_command "$test_cmd" "$expected_result"
      done

      printf "* configured with correct service"
      test_cmd="kubectl get mutatingwebhookconfiguration webhook.knative.dev -o jsonpath={.webhooks[].clientConfig.service.name}"
      expected_result="webhook"
      test_command "$test_cmd" "$expected_result"

      printf "* configured with correct namespace"
      test_cmd="kubectl get mutatingwebhookconfiguration webhook.knative.dev -o jsonpath={.webhooks[].clientConfig.service.namespace}"
      expected_result="knative-serving"
      test_command "$test_cmd" "$expected_result"
      ;;
    controllers_running)
      printf "Knative Serving controllers are running"
      test_cmd="kubectl get pods -n knative-serving -l app=controller -o jsonpath={.items[].status.phase}"
      expected_result="Running"
      test_command "$test_cmd" "$expected_result"
      ;;
    istio_installed)
      istio_components=(mixer ingress pilot istio-ca)
      for component in ${istio_components[*]}; do
        printf "Istio $component is installed"
        test_cmd="kubectl get pods -n istio-system -l istio=$component -o jsonpath={.items[].status.phase}"
        expected_result="Running"
        test_command "$test_cmd" "$expected_result"
      done
      ;;
    *)
      echo "Unknown test case: $test_name"
      exit 1
  esac
}

echo "Running diagnose-me..."
for test in ${tests[*]}; do
  run_test "$test"
done
