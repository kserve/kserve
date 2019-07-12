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

source $(dirname $0)/../vendor/github.com/knative/test-infra/scripts/release.sh

function build_release() {
  # Run `generate-yamls.sh`, which should be versioned with the
  # branch since the detail of building may change over time.
  local YAML_LIST="$(mktemp)"
  # We publish our own istio.yaml, so users don't need to use helm
  $(dirname $0)/generate-yamls.sh "${REPO_ROOT_DIR}" "${YAML_LIST}"
  YAMLS_TO_PUBLISH=$(cat "${YAML_LIST}" | tr '\n' ' ')
  if (( ! PUBLISH_RELEASE )); then
    # Copy the generated YAML files to the repo root dir if not publishing.
    cp ${YAMLS_TO_PUBLISH} ${REPO_ROOT_DIR}
  fi
}

main $@
