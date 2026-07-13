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

while IFS= read -r -d '' file
do
  if ! grep -q Copyright "$file"
    then
      cat hack/boilerplate.go.txt "$file" > "$file".new && mv "$file".new "$file"
    fi
done <   <(find ./pkg ./cmd -name '*.go' -print0)

while IFS= read -r -d '' file
do
  if ! grep -q Copyright "$file"
    then
      cat hack/boilerplate.python.txt "$file" > "$file".new && mv "$file".new "$file"
    fi
done <   <(find ./python -name '*.py' -print0)
