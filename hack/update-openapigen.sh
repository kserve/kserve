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

# find_project_root [start_dir] [marker]
#   start_dir : directory to begin the search (defaults to this script’s dir)
#   marker    : filename or directory name to look for (defaults to "go.mod")
#
# Prints the first dir containing the marker, or exits 1 if none found.
find_project_root() {
  local start_dir="${1:-$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)}"
  local marker="${2:-go.mod}"
  local dir="$start_dir"

  while [[ "$dir" != "/" && ! -e "$dir/$marker" ]]; do
    dir="$(dirname "$dir")"
  done

  if [[ -e "$dir/$marker" ]]; then
    printf '%s\n' "$dir"
  else
    echo "Error: couldn’t find '$marker' in any parent of '$start_dir'" >&2
    return 1
  fi
}

# Find project root and change to it, so all subsequent paths are relative to the root.
PROJECT_ROOT="$(find_project_root)"
cd "$PROJECT_ROOT"
echo "Running commands from project root: $PROJECT_ROOT"

KNOWN_VIOLATION_EXCEPTIONS=hack/violation_exceptions.list
CURRENT_VIOLATION_EXCEPTIONS=hack/current_violation_exceptions.list
OPENAPI_SPEC_FILE=pkg/openapi/openapi_generated.go

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

echo "Successfully updated OpenAPI specs and swagger.json."