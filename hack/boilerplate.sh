#!/bin/bash

# Copyright 2023 The Kubeflow Authors.
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

# This script adds or updates KServe copyright headers in Python and Go files.
# - Files with no copyright header receive a new header with the current year.
# - Files with a stale "The KServe Authors" copyright year are updated to the
#   current year.
# - Files with non-KServe copyright (e.g. Kubeflow) are left unchanged.

CURRENT_YEAR=$(date +%Y)

# Process Go files under pkg/ and cmd/
while IFS= read -r -d '' file
do
  if ! grep -q "Copyright" "$file"; then
    # No copyright header at all — add one with the current year
    sed "s/Copyright [0-9]\{4\}/Copyright ${CURRENT_YEAR}/" hack/boilerplate.go.txt > "${file}.new"
    cat "${file}.new" "${file}" > "${file}.new2"
    mv "${file}.new2" "${file}"
    rm -f "${file}.new"
  elif grep -q "The KServe Authors" "$file" && ! grep -q "Copyright ${CURRENT_YEAR} The KServe Authors" "$file"; then
    # KServe copyright header exists but the year is stale — update it
    sed -i'.bak' "s/Copyright [0-9]\{4\} The KServe Authors\./Copyright ${CURRENT_YEAR} The KServe Authors./" "$file"
    rm -f "${file}.bak"
  fi
done < <(find ./pkg ./cmd -name '*.go' -print0)

# Process Python files under python/
while IFS= read -r -d '' file
do
  if ! grep -q "Copyright" "$file"; then
    # No copyright header at all — add one with the current year
    sed "s/Copyright [0-9]\{4\}/Copyright ${CURRENT_YEAR}/" hack/boilerplate.python.txt > "${file}.new"
    cat "${file}.new" "${file}" > "${file}.new2"
    mv "${file}.new2" "${file}"
    rm -f "${file}.new"
  elif grep -q "The KServe Authors" "$file" && ! grep -q "Copyright ${CURRENT_YEAR} The KServe Authors" "$file"; then
    # KServe copyright header exists but the year is stale — update it
    sed -i'.bak' "s/# Copyright [0-9]\{4\} The KServe Authors\./# Copyright ${CURRENT_YEAR} The KServe Authors./" "$file"
    rm -f "${file}.bak"
  fi
done < <(find ./python -name '*.py' -print0)
