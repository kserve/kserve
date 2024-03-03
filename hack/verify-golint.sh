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

set -o errexit
set -o pipefail

# golangci-lint binary path
golangci_lint_binary="$(go env GOPATH)/bin/golangci-lint"

# Check if golangci-lint is already installed
if ! command -v golangci-lint &> /dev/null; then
    echo "installing golangci-lint"
    go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.55.2
	# Verify golangci-lint installation
	$golangci_lint_binary --version
fi

# Run golangci-lint
$golangci_lint_binary run
