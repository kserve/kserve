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

if [ -z "${GOROOT:-}" ]; then
    export GOROOT=$(go env GOROOT)
fi

# Below hacks are to add "// +k8s:openapi-gen=true" for some dependencies.
# The knative related hacking can be removed once upgraded (knative/pkg/pull/510).
knative_condition_file="vendor/github.com/knative/pkg/apis/condition_types.go"
knative_volatile_file="vendor/github.com/knative/pkg/apis/volatile_time.go"
knative_url_file="vendor/github.com/knative/pkg/apis/url.go"
net_url_file="${GOROOT}/src/net/url/url.go"

function add_openapi_tag()
{
    file=$1
    refer=$2
    if ! grep -q "+k8s:openapi-gen=true" ${file} ;then
            sed -i "/^${refer}/i // +k8s:openapi-gen=true" ${file}
    fi
}

add_openapi_tag ${knative_condition_file} "type Condition struct {"
add_openapi_tag ${knative_volatile_file} "type VolatileTime struct {"
add_openapi_tag ${knative_url_file} "type URL url.URL"
add_openapi_tag ${net_url_file} "type Userinfo struct {"

# Generating OpenAPI specification
go run vendor/k8s.io/code-generator/cmd/openapi-gen/main.go --input-dirs github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1,github.com/knative/pkg/apis,net/url --output-package github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1/ --go-header-file hack/boilerplate.go.txt

# Workaroud error "spec redeclared in this block" in unit test.
sed -i 's%spec "github.com/go-openapi/spec"%openapispec "github.com/go-openapi/spec"%g' pkg/apis/serving/v1alpha1/openapi_generated.go
sed -i 's/spec\./openapispec\./g' pkg/apis/serving/v1alpha1/openapi_generated.go

# Generating swagger file
go run cmd/spec-gen/main.go 0.1 > pkg/apis/serving/v1alpha1/swagger.json

