#!/bin/bash

# Copyright 2023 The KServe Authors.
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

# The script will install knative operator in the GH Actions environment.

set -o errexit
set -o nounset
set -o pipefail

KNATIVE_OPERATOR_VERSION="v1.16.0"

echo "Installing Knative Operator ..."
helm install knative-operator --namespace knative-operator --create-namespace --wait \
      https://github.com/knative/operator/releases/download/knative-${KNATIVE_OPERATOR_VERSION}/knative-operator-${KNATIVE_OPERATOR_VERSION}.tgz
