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
set -o nounset
set -o pipefail

if [ -z "${GOPATH:-}" ]; then
    export GOPATH=$(go env GOPATH)
fi

KNOWN_VIOLATION_EXCEPTIONS=hack/violation_exceptions.list
CURRENT_VIOLATION_EXCEPTIONS=hack/current_violation_exceptions.list

# Generating OpenAPI specification
go run vendor/k8s.io/kube-openapi/cmd/openapi-gen/openapi-gen.go \
    --input-dirs github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha2,knative.dev/pkg/apis,knative.dev/pkg/apis/duck/v1beta1 \
    --output-package github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha2/ --go-header-file hack/boilerplate.go.txt \
    -r $CURRENT_VIOLATION_EXCEPTIONS

test -f $CURRENT_VIOLATION_EXCEPTIONS || touch $CURRENT_VIOLATION_EXCEPTIONS

# The API rule fails if generated API rule violation report differs from the
# checked-in violation file, prints error message to request developer to
# fix either the API source code, or the known API rule violation file.
diff $CURRENT_VIOLATION_EXCEPTIONS $KNOWN_VIOLATION_EXCEPTIONS || \
    (echo -e "ERROR: \n\t API rule check failed. Reported violations in file $CURRENT_VIOLATION_EXCEPTIONS differ from known violations in file $KNOWN_VIOLATION_EXCEPTIONS. \n"; exit 1)

# Generating swagger file
go run cmd/spec-gen/main.go 0.1 > pkg/apis/serving/v1alpha2/swagger.json
