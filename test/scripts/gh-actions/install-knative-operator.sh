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

KNATIVE_OPERATOR_PLUGIN_VERSION="knative-v1.13.0"
KNATIVE_OPERATOR_VERSION="1.13.1"
KNATIVE_CLI_VERSION="knative-v1.13.0"

echo "Installing Knative cli ..."
wget https://github.com/knative/client/releases/download/"${KNATIVE_CLI_VERSION}"/kn-linux-amd64 -O /usr/local/bin/kn && chmod +x /usr/local/bin/kn

echo "Installing Knative Operator ..."
wget https://github.com/knative-sandbox/kn-plugin-operator/releases/download/"${KNATIVE_OPERATOR_PLUGIN_VERSION}"/kn-operator-linux-amd64 -O kn-operator && chmod +x kn-operator
mkdir -p ~/.config/kn/plugins
mv kn-operator ~/.config/kn/plugins
kn operator install -n knative-operator -v "${KNATIVE_OPERATOR_VERSION}"
kubectl wait --for=condition=Ready pods --all --timeout=300s -n knative-operator
