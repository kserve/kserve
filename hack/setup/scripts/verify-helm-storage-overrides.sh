#!/usr/bin/env bash

# Copyright 2026 The KServe Authors.
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

set -euo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel)"

EXPECTED_MEMORY_REQUEST="2Gi"
EXPECTED_MEMORY_LIMIT="32Gi"
EXPECTED_CPU_REQUEST="500m"
EXPECTED_CPU_LIMIT="2"

fail() {
  echo "[ERROR] $*" >&2
  exit 1
}

assert_contains() {
  local haystack="$1"
  local needle="$2"
  local context="$3"

  if ! grep -Fq "$needle" <<< "$haystack"; then
    fail "Missing expected value in ${context}: ${needle}"
  fi
}

render_chart() {
  local chart="$1"

  helm template test "${REPO_ROOT}/charts/${chart}" \
    --set "kserve.storage.resources.limits.memory=${EXPECTED_MEMORY_LIMIT}" \
    --set "kserve.storage.resources.requests.memory=${EXPECTED_MEMORY_REQUEST}" \
    --set "kserve.storage.resources.requests.cpu=${EXPECTED_CPU_REQUEST}" \
    --set "kserve.storage.resources.limits.cpu=${EXPECTED_CPU_LIMIT}"
}

extract_configmap_doc() {
  awk '
    BEGIN { doc = "" }
    /^[[:space:]]*---[[:space:]]*$/ {
      if (doc ~ /kind:[[:space:]]*ConfigMap/ && doc ~ /name:[[:space:]]*inferenceservice-config/) {
        printf "%s", doc
        exit
      }
      doc = ""
      next
    }
    {
      doc = doc $0 "\\n"
    }
    END {
      if (doc ~ /kind:[[:space:]]*ConfigMap/ && doc ~ /name:[[:space:]]*inferenceservice-config/) {
        printf "%s", doc
      }
    }
  '
}

extract_csc_doc() {
  awk '
    BEGIN { doc = "" }
    /^[[:space:]]*---[[:space:]]*$/ {
      if (doc ~ /kind:[[:space:]]*ClusterStorageContainer/) {
        printf "%s", doc
        exit
      }
      doc = ""
      next
    }
    {
      doc = doc $0 "\\n"
    }
    END {
      if (doc ~ /kind:[[:space:]]*ClusterStorageContainer/) {
        printf "%s", doc
      }
    }
  '
}

verify_chart() {
  local chart="$1"
  local rendered
  local configmap_doc
  local csc_doc

  echo "Checking ${chart}..."
  rendered="$(render_chart "$chart")"

  configmap_doc="$(printf "%s" "$rendered" | extract_configmap_doc)"
  [[ -n "$configmap_doc" ]] || fail "Could not find inferenceservice-config ConfigMap in ${chart}"

  assert_contains "$configmap_doc" "\"memoryRequest\": \"${EXPECTED_MEMORY_REQUEST}\"" "${chart} inferenceservice-config.storageInitializer"
  assert_contains "$configmap_doc" "\"memoryLimit\": \"${EXPECTED_MEMORY_LIMIT}\"" "${chart} inferenceservice-config.storageInitializer"
  assert_contains "$configmap_doc" "\"cpuRequest\": \"${EXPECTED_CPU_REQUEST}\"" "${chart} inferenceservice-config.storageInitializer"
  assert_contains "$configmap_doc" "\"cpuLimit\": \"${EXPECTED_CPU_LIMIT}\"" "${chart} inferenceservice-config.storageInitializer"

  csc_doc="$(printf "%s" "$rendered" | extract_csc_doc)"
  [[ -n "$csc_doc" ]] || fail "Could not find ClusterStorageContainer in ${chart}"

  assert_contains "$csc_doc" "memory: ${EXPECTED_MEMORY_LIMIT}" "${chart} ClusterStorageContainer.resources.limits"
  assert_contains "$csc_doc" "cpu: ${EXPECTED_CPU_LIMIT}" "${chart} ClusterStorageContainer.resources.limits"
  assert_contains "$csc_doc" "memory: ${EXPECTED_MEMORY_REQUEST}" "${chart} ClusterStorageContainer.resources.requests"
  assert_contains "$csc_doc" "cpu: ${EXPECTED_CPU_REQUEST}" "${chart} ClusterStorageContainer.resources.requests"

  echo "  OK"
}

echo "Verifying Helm storage override propagation..."
verify_chart "kserve-resources"
verify_chart "kserve-llmisvc-resources"
echo "All assertions passed"
