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

# This script adds copyright to the python and go files.

CURRENT_YEAR=$(date +%Y)

# Returns the year the file was first committed, or the current year for untracked files.
file_year() {
  local year
  year=$(git log --follow --diff-filter=A --format=%ad --date=format:%Y -- "$1" 2>/dev/null | tail -1)
  echo "${year:-$CURRENT_YEAR}"
}

while IFS= read -r -d '' file
do
  if ! grep -q Copyright "$file"
    then
      local_year=$(file_year "$file")
      sed "s/ YEAR/ ${local_year}/g" hack/boilerplate.go.txt | cat - "$file" > "$file".new && mv "$file".new "$file"
    fi
done <   <(find ./pkg ./cmd -name '*.go' -print0)

while IFS= read -r -d '' file
do
  if ! grep -q Copyright "$file"
    then
      local_year=$(file_year "$file")
      sed "s/ YEAR/ ${local_year}/g" hack/boilerplate.python.txt | cat - "$file" > "$file".new && mv "$file".new "$file"
    fi
done <   <(find ./python ./test/e2e -name '*.py' -not -name '*_pb2*.py' -print0)
