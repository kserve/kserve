#!/usr/bin/env bash
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -euo pipefail

# get_openshift_server_version
#   Extracts the Server Version from 'oc version' output
#   Returns the version string (e.g., "4.19.9") or exits with error if not found
get_openshift_server_version() {
  local version_output
  local server_version

  if ! version_output=$(oc version 2>/dev/null); then
    echo "Error: Failed to execute 'oc version'. Make sure oc is installed and you're logged in to OpenShift." >&2
    return 1
  fi

  if server_version=$(echo "$version_output" | grep "Server Version:" | awk '{print $3}'); then
    if [ -n "$server_version" ]; then
      echo "$server_version"
      return 0
    fi
  fi

  echo "Error: Could not find Server Version in 'oc version' output." >&2
  echo "oc version output:" >&2
  echo "$version_output" >&2
  return 1
}

# version_compare <version1> <version2>
#   Compares two version strings in semantic version format (e.g., "4.19.9")
#   Nightly versions (e.g., "4.19.0-0.nightly-...") automatically pass
#   Returns 0 if version1 >= version2, 1 otherwise
version_compare() {
  local version1="$1"
  local version2="$2"

  # Nightly builds always pass the version check
  if [[ "$version1" == *"nightly"* ]]; then
    return 0
  fi

  local v1=$(echo "$version1" | awk -F. '{printf "%d%03d%03d", $1, $2, $3}')
  local v2=$(echo "$version2" | awk -F. '{printf "%d%03d%03d", $1, $2, $3}')

  [ "$v1" -ge "$v2" ]
}
