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

KNOWN_VIOLATION_EXCEPTIONS=hack/violation_exceptions.list
CURRENT_VIOLATION_EXCEPTIONS=hack/current_violation_exceptions.list
OPENAPI_SPEC_FILE=pkg/openapi/openapi_generated.go

GOPATH=$(go env GOPATH)
if [[ -z $GOPATH ]]
then
    echo "Error: GOPATH is not set. Please configure your GOPATH environment variable."
    exit 1
fi    
TARGET_DIR="$GOPATH/src/github.com/kserve/kserve"
CURRENT_DIR=$(pwd)

# Check if the current directory is the target directory
if [[ "$CURRENT_DIR" != "$TARGET_DIR" ]]; then
    echo "You are not in the target directory: $TARGET_DIR"

    # Check if the target directory exists.
    if [[ -d "$TARGET_DIR" ]]; then    
        mv $TARGET_DIR "${TARGET_DIR}_$(date +%Y%m%d_%H%M%S)"
    fi
    
    echo "Creating a symbolic link for the target directory ..."
    mkdir -p "$(dirname "$TARGET_DIR")"
    ln -s "$CURRENT_DIR" "$TARGET_DIR"

    # Change to the target directory
    echo "Changing to the target directory: $TARGET_DIR"
    pushd "$TARGET_DIR" > /dev/null
fi

# Generating OpenAPI specification
go run k8s.io/kube-openapi/cmd/openapi-gen \
    --output-pkg github.com/kserve/kserve/pkg/openapi --output-dir "./pkg/openapi" \
    --output-file "openapi_generated.go" \
    -v 5 --go-header-file hack/boilerplate.go.txt \
    -r $CURRENT_VIOLATION_EXCEPTIONS \
    "knative.dev/pkg/apis" \
    "knative.dev/pkg/apis/duck/v1" \
    "./pkg/apis/serving/v1beta1" \
    "./pkg/apis/serving/v1alpha1"

# Hack, the name is required in openAPI specification even if set "+optional" for v1.Container in PredictorExtensionSpec.
sed -i'.bak' -e 's/Required: \[\]string{\"name\"},//g' $OPENAPI_SPEC_FILE && rm -rf $OPENAPI_SPEC_FILE.bak
sed -i'.bak' -e 's/Required: \[\]string{\"modelFormat\", \"name\"},/Required: \[\]string{\"modelFormat\"},/g' $OPENAPI_SPEC_FILE && rm -rf $OPENAPI_SPEC_FILE.bak

test -f $CURRENT_VIOLATION_EXCEPTIONS || touch $CURRENT_VIOLATION_EXCEPTIONS

# The API rule fails if generated API rule violation report differs from the
# checked-in violation file, prints error message to request developer to
# fix either the API source code, or the known API rule violation file.
diff $CURRENT_VIOLATION_EXCEPTIONS $KNOWN_VIOLATION_EXCEPTIONS || \
    (echo -e "ERROR: \n\t API rule check failed. Reported violations in file $CURRENT_VIOLATION_EXCEPTIONS differ from known violations in file $KNOWN_VIOLATION_EXCEPTIONS. \n"; exit 1)

# Generating swagger file
go run cmd/spec-gen/main.go 0.1 > pkg/openapi/swagger.json

# Return to the original directory
if [[ "$CURRENT_DIR" != "$TARGET_DIR" ]]; then
    echo "Returning to the original directory: $CURRENT_DIR"
    popd > /dev/null
fi
